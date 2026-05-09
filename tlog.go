package tlog

import (
	"io"
	"os"
	"sync"
	"time"
	"unsafe"

	"nikand.dev/go/hacked/htime"
	"tlog.app/go/loc"

	"tlog.app/go/tlog/tlwire"
)

type (
	// Logger encodes structured events and writes them to underlaying [io.Writer].
	// Nil Logger is a valid state, which ignores all the events.
	Logger struct {
		io.Writer // protected by Mutex below

		tlwire.Encoder

		// NowID customizes id generation.
		NewID func() ID `deep:"compare=pointer"` // must be threadsafe

		now  func() time.Time `deep:"compare=pointer"`
		nano func() int64     `deep:"compare=pointer"`

		callers     func(skip int, pc *loc.PC, len, cap int) int `deep:"compare=pointer"`
		callersSkip int

		filter *filter // atomic access

		sync.Mutex

		b  []byte
		ls []byte
	}

	// Span is a tracing span.
	// It's essentially a [Logger] + span [ID].
	// Events logged with the same Span have the same spanID,
	// thus grouping them.
	Span struct {
		Logger    *Logger
		ID        ID
		StartedAt time.Time
	}

	LogLevel int

	// EventKind classifies event kind: span_start, span_end, untyped_event.
	EventKind rune

	// for like stdlib log.SetOutput(l).
	writeWrapper struct {
		Span

		d int
	}

	dumpWrapper struct {
		Span

		d   int
		msg string
		key string

		ctx []byte

		buf [24]byte
	}
)

var (
	Stdout = os.Stdout
	Stderr = os.Stderr
)

// Log levels.
const (
	Info LogLevel = iota
	Warn
	Error
	Fatal

	Debug LogLevel = -1
)

// Predefined canonical keys.
var (
	KeySpan      = "_s"
	KeyParent    = "_p"
	KeyTimestamp = "_t"
	KeyElapsed   = "_e"
	KeyCaller    = "_c"
	KeyMessage   = "_m"
	KeyEventKind = "_k"
	KeyLogLevel  = "_l"
	KeyRepeated  = "_r"
	KeyTag       = "_T"
)

// Event kinds.
const (
	EventSpanStart  EventKind = 's'
	EventSpanFinish EventKind = 'f'
	EventMetric     EventKind = 'm'
)

// DefaultLogger is a global logger used by package level functions.
var DefaultLogger = New(NewConsoleWriter(os.Stderr, LdetFlags))

// Root wraps default logger into span without ID (kinda casts type).
func Root() Span { return Span{Logger: DefaultLogger} }

// Root wraps Logger into Span (kinda casts type).
func (l *Logger) Root() Span { return Span{Logger: l} }

// New creates new [Logger].
func New(w io.Writer) *Logger {
	return &Logger{
		Writer:  w,
		NewID:   MathRandID,
		now:     time.Now,
		nano:    htime.UnixNano,
		callers: caller1,
	}
}

func (l *Logger) Copy(w io.Writer) *Logger {
	return &Logger{
		Writer:      w,
		Encoder:     l.Encoder,
		NewID:       l.NewID,
		now:         l.now,
		nano:        l.nano,
		callers:     l.callers,
		callersSkip: l.callersSkip,
		filter:      l.getfilter(),
	}
}

func (s Span) Copy(w io.Writer) Span {
	return Span{
		Logger:    s.Logger.Copy(w),
		ID:        s.ID,
		StartedAt: s.StartedAt,
	}
}

func message(l *Logger, id ID, d int, msg any, kvs []any) {
	if l == nil {
		return
	}

	e := &l.Encoder

	defer l.Unlock()
	l.Lock()

	l.b = e.AppendMap(l.b[:0], -1)

	if id != (ID{}) {
		l.b = e.AppendString(l.b, KeySpan)
		l.b = id.TlogAppend(l.b)
	}

	if l.nano != nil {
		now := l.nano()

		l.b = e.AppendString(l.b, KeyTimestamp)
		l.b = e.AppendTimestamp(l.b, now)
	}

	var c loc.PC

	if d >= 0 && l.callers != nil && l.callers(2+d+l.callersSkip, (*loc.PC)(noescape(unsafe.Pointer(&c))), 1, 1) != 0 {
		l.b = e.AppendKey(l.b, KeyCaller)
		l.b = e.AppendCaller(l.b, c)
	}

	if msg != nil {
		l.b = e.AppendKey(l.b, KeyMessage)
		l.b = e.AppendSemantic(l.b, WireMessage)

		switch msg := msg.(type) {
		case string:
			l.b = e.AppendString(l.b, msg)
		case []byte:
			l.b = e.AppendTagBytes(l.b, tlwire.String, msg)
		case format:
			l.b = e.AppendFormatf(l.b, msg.Fmt, msg.Args...)
		default:
			l.b = e.AppendFormat(l.b, msg)
		}
	}

	l.b = AppendKVs(e, l.b, kvs)

	l.b = append(l.b, l.ls...)

	l.b = e.AppendBreak(l.b)

	_, _ = l.Writer.Write(l.b)
}

func newspan(l *Logger, par ID, d int, n string, kvs []any) (s Span) {
	if l == nil {
		return
	}

	s.Logger = l
	s.ID = l.NewID()
	if l.now != nil {
		s.StartedAt = l.now()
	}

	e := &l.Encoder

	defer l.Unlock()
	l.Lock()

	l.b = e.AppendMap(l.b[:0], -1)

	l.b = e.AppendString(l.b, KeySpan)
	l.b = s.ID.TlogAppend(l.b)

	if l.now != nil {
		l.b = e.AppendString(l.b, KeyTimestamp)
		l.b = e.AppendTimestamp(l.b, s.StartedAt.UnixNano())
	}

	if d >= 0 {
		var c loc.PC
		caller1(2+d, &c, 1, 1)

		l.b = e.AppendKey(l.b, KeyCaller)
		l.b = e.AppendCaller(l.b, c)
	}

	l.b = e.AppendString(l.b, KeyEventKind)
	l.b = EventSpanStart.TlogAppend(l.b)

	if par != (ID{}) {
		l.b = e.AppendString(l.b, KeyParent)
		l.b = par.TlogAppend(l.b)
	}

	if n != "" {
		l.b = e.AppendString(l.b, KeyMessage)
		l.b = e.AppendSemantic(l.b, WireMessage)
		l.b = e.AppendString(l.b, n)
	}

	l.b = AppendKVs(e, l.b, kvs)

	l.b = append(l.b, l.ls...)

	l.b = e.AppendBreak(l.b)

	_, _ = l.Writer.Write(l.b)

	return
}

// Finish emits finish event adding provided key-values pairs.
func (s Span) Finish(kvs ...any) {
	if s.Logger == nil {
		return
	}

	l := s.Logger
	e := &l.Encoder

	defer l.Unlock()
	l.Lock()

	l.b = e.AppendTag(l.b[:0], tlwire.Map, -1)

	if s.ID != (ID{}) {
		l.b = e.AppendString(l.b, KeySpan)
		l.b = s.ID.TlogAppend(l.b)
	}

	var now time.Time
	if l.now != nil {
		now = l.now()

		l.b = e.AppendString(l.b, KeyTimestamp)
		l.b = e.AppendTimestamp(l.b, now.UnixNano())
	}

	l.b = e.AppendString(l.b, KeyEventKind)
	l.b = EventSpanFinish.TlogAppend(l.b)

	if l.now != nil {
		l.b = e.AppendString(l.b, KeyElapsed)
		l.b = e.AppendDuration(l.b, now.Sub(s.StartedAt))
	}

	l.b = AppendKVs(e, l.b, kvs)

	l.b = append(l.b, l.ls...)

	l.b = e.AppendBreak(l.b)

	_, _ = l.Writer.Write(l.b)
}

// SetLabels sets static key-value pairs, that are appended to each event.
func SetLabels(kvs ...interface{}) {
	DefaultLogger.SetLabels(kvs...)
}

// SetLabels sets static key-value pairs, that are appended to each event.
func (l *Logger) SetLabels(kvs ...interface{}) {
	if l == nil {
		return
	}

	defer l.Unlock()
	l.Lock()

	l.ls = AppendLabels(&l.Encoder, l.ls[:0], kvs)
}

func (l *Logger) Labels() RawMessage {
	return l.ls
}

// Start starts a new Span.
// It emit start event and returns its id, so subsequent events are assigned the same span [ID].
func Start(name string, kvs ...any) Span {
	return newspan(DefaultLogger, ID{}, 0, name, kvs)
}

// Or returns `l` if it's not nil, or `l2`.
func (l *Logger) Or(l2 *Logger) *Logger {
	if l != nil {
		return l
	}

	return l2
}

// Or returns `s` if its Logger is not nil, or `s2`.
func (s Span) Or(s2 Span) Span {
	if s.Logger != nil {
		return s
	}

	return s2
}

// Event emits generic event without adding default keys, such as time and caller.
func (l *Logger) Event(kvs ...any) (err error) {
	if l == nil {
		return nil
	}

	defer l.Unlock()
	l.Lock()

	l.b = l.AppendMap(l.b[:0], -1)

	l.b = AppendKVs(&l.Encoder, l.b, kvs)

	l.b = append(l.b, l.ls...)

	l.b = l.AppendBreak(l.b)

	_, err = l.Writer.Write(l.b)

	return
}

// Event emits generic event without adding default keys, such as time and caller.
func (s Span) Event(kvs ...interface{}) (err error) {
	if s.Logger == nil {
		return nil
	}

	l := s.Logger
	e := &l.Encoder

	defer l.Unlock()
	l.Lock()

	l.b = l.AppendMap(l.b[:0], -1)

	if s.ID != (ID{}) {
		l.b = l.AppendString(l.b, KeySpan)
		l.b = s.ID.TlogAppend(l.b)
	}

	l.b = AppendKVs(e, l.b, kvs)

	l.b = append(l.b, l.ls...)

	l.b = l.AppendBreak(l.b)

	_, err = l.Writer.Write(l.b)

	return
}

// NewSpan is a general form of [Start]/[Spawn] if caller depth or parent ID need to be customized.
func (l *Logger) NewSpan(d int, par ID, name string, kvs ...any) Span {
	return newspan(l, par, d, name, kvs)
}

// NewMessage is a general form of [Printw] if caller depth id span ID need to be customized.
func (l *Logger) NewMessage(d int, id ID, msg any, kvs ...any) {
	message(l, id, d, msg, kvs)
}

// NewMessage is a general form of [Printw] if caller depth needs to be customized.
func (s Span) NewMessage(d int, msg any, kvs ...any) {
	message(s.Logger, s.ID, d, msg, kvs)
}

// Start emits start event with given name and key-value pairs.
// Generated span has no parent.
func (l *Logger) Start(name string, kvs ...any) Span {
	return newspan(l, ID{}, 0, name, kvs)
}

// Spawn emits start event with given name and key-value pairs.
// Generated span is a child of `s`.
func (s Span) Spawn(name string, kvs ...any) Span {
	return newspan(s.Logger, s.ID, 0, name, kvs)
}

// Printw emits event with message and and an array or key-values pairs.
func Printw(msg string, kvs ...any) {
	message(DefaultLogger, ID{}, 0, msg, kvs)
}

// Printw emits event with message and and an array or key-values pairs.
func (l *Logger) Printw(msg string, kvs ...any) {
	message(l, ID{}, 0, msg, kvs)
}

// Printw emits event with message and and an array or key-values pairs.
func (s Span) Printw(msg string, kvs ...any) {
	message(s.Logger, s.ID, 0, msg, kvs)
}

// Printf emits event similar to log.Printf.
func Printf(fmt string, args ...any) {
	message(DefaultLogger, ID{}, 0, format{Fmt: fmt, Args: args}, nil)
}

// Printf emits event similar to log.Printf.
func (l *Logger) Printf(fmt string, args ...any) {
	message(l, ID{}, 0, format{Fmt: fmt, Args: args}, nil)
}

// Printf emits event similar to log.Printf.
func (s Span) Printf(fmt string, args ...any) {
	message(s.Logger, s.ID, 0, format{Fmt: fmt, Args: args}, nil)
}

// IOWriter creates io.Writer, which emits one event per one Write.
// It's suitable to provide it to other logger.
func (l *Logger) IOWriter(d int) io.Writer {
	return writeWrapper{
		Span: Span{
			Logger: l,
		},
		d: d,
	}
}

// IOWriter creates io.Writer, which emits one event per one Write.
// It's suitable to provide it to other logger.
func (s Span) IOWriter(d int) io.Writer {
	return writeWrapper{
		Span: s,
		d:    d,
	}
}

// DumpWriter creates io.Writer, similar to IOWriter,
// but bytes are written as `key` value instead of message.
func (l *Logger) DumpWriter(d int, msg, key string, kvs ...any) io.Writer {
	return Span{Logger: l}.DumpWriter(d, msg, key, kvs...)
}

// DumpWriter creates io.Writer, similar to IOWriter,
// but bytes are written as `key` value instead of message.
func (s Span) DumpWriter(d int, msg, key string, kvs ...any) io.Writer {
	w := &dumpWrapper{
		Span: s,

		d:   d,
		msg: msg,
		key: key,
	}

	w.ctx = AppendKVs(&s.Logger.Encoder, w.buf[:0], kvs)

	return w
}

func (w writeWrapper) Write(p []byte) (int, error) {
	message(w.Logger, w.ID, w.d, p, nil)

	return len(p), nil
}

func (w *dumpWrapper) Write(p []byte) (int, error) {
	message(w.Logger, w.ID, w.d, w.msg, []any{w, w.key, p})

	return len(p), nil
}

func (w *dumpWrapper) TlogAppend(b []byte) []byte {
	return append(b, w.ctx...)
}

// Write implements [io.Writer].
// p is expected to be tlog encoded event(s).
// It acts as [io.Writer] for the other [Logger] for some reason.
func (l *Logger) Write(p []byte) (int, error) {
	if l == nil || l.Writer == nil {
		return len(p), nil
	}

	defer l.Unlock()
	l.Lock()

	return l.Writer.Write(p)
}

// OK returns true if logger is not nil.
func (l *Logger) OK() bool { return l != nil }

// OK returns true if underlaying logger is not nil.
func (s Span) OK() bool { return s.Logger != nil }

// LoggerSetTimeNow overrides standard [time.Now] and [time,Now().UnixNano()]
// or disables adding time to events if nil provided.
// Mostly used for testing.
func LoggerSetTimeNow(l *Logger, now func() time.Time, nano func() int64) {
	l.now = now
	l.nano = nano
}

// LoggerSetCallers overrides standard [runtime.Caller]
// or disables adding caller info to events if nil provided.
func LoggerSetCallers(l *Logger, skip int, callers func(skip int, pc []uintptr) int) {
	l.callers = *(*func(int, *loc.PC, int, int) int)(unsafe.Pointer(&callers))
	l.callersSkip = skip + 1
}
