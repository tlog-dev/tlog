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
	Logger struct {
		io.Writer // protected by Mutex below

		tlwire.Encoder

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

	Span struct {
		Logger    *Logger
		ID        ID
		StartedAt time.Time
	}

	LogLevel int

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

// Predefined keys.
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
)

// Event kinds.
const (
	EventSpanStart  EventKind = 's'
	EventSpanFinish EventKind = 'f'
	EventMetric     EventKind = 'm'
)

var DefaultLogger = New(NewConsoleWriter(os.Stderr, LdetFlags))

func Root() Span { return Span{Logger: DefaultLogger} }

func (l *Logger) Root() Span { return Span{Logger: l} }

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

func message(l *Logger, id ID, d int, msg interface{}, kvs []interface{}) {
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

func newspan(l *Logger, par ID, d int, n string, kvs []interface{}) (s Span) {
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

func (s Span) Finish(kvs ...interface{}) {
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

func SetLabels(kvs ...interface{}) {
	DefaultLogger.SetLabels(kvs...)
}

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

func Start(name string, kvs ...interface{}) Span {
	return newspan(DefaultLogger, ID{}, 0, name, kvs)
}

func (l *Logger) Or(l2 *Logger) *Logger {
	if l != nil {
		return l
	}

	return l2
}

func (s Span) Or(s2 Span) Span {
	if s.Logger != nil {
		return s
	}

	return s2
}

func (l *Logger) Event(kvs ...interface{}) (err error) {
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

func (l *Logger) NewSpan(d int, par ID, name string, kvs ...interface{}) Span {
	return newspan(l, par, d, name, kvs)
}

func (l *Logger) NewMessage(d int, id ID, msg interface{}, kvs ...interface{}) {
	message(l, id, d, msg, kvs)
}

func (s Span) NewMessage(d int, msg interface{}, kvs ...interface{}) {
	message(s.Logger, s.ID, d, msg, kvs)
}

func (l *Logger) Start(name string, kvs ...interface{}) Span {
	return newspan(l, ID{}, 0, name, kvs)
}

func (s Span) Spawn(name string, kvs ...interface{}) Span {
	return newspan(s.Logger, s.ID, 0, name, kvs)
}

func Printw(msg string, kvs ...interface{}) {
	message(DefaultLogger, ID{}, 0, msg, kvs)
}

func (l *Logger) Printw(msg string, kvs ...interface{}) {
	message(l, ID{}, 0, msg, kvs)
}

func (s Span) Printw(msg string, kvs ...interface{}) {
	message(s.Logger, s.ID, 0, msg, kvs)
}

func Printf(fmt string, args ...interface{}) {
	message(DefaultLogger, ID{}, 0, format{Fmt: fmt, Args: args}, nil)
}

func (l *Logger) Printf(fmt string, args ...interface{}) {
	message(l, ID{}, 0, format{Fmt: fmt, Args: args}, nil)
}

func (s Span) Printf(fmt string, args ...interface{}) {
	message(s.Logger, s.ID, 0, format{Fmt: fmt, Args: args}, nil)
}

func (l *Logger) IOWriter(d int) io.Writer {
	return writeWrapper{
		Span: Span{
			Logger: l,
		},
		d: d,
	}
}

func (s Span) IOWriter(d int) io.Writer {
	return writeWrapper{
		Span: s,
		d:    d,
	}
}

func (l *Logger) DumpWriter(d int, msg, key string, kvs ...any) io.Writer {
	return Span{Logger: l}.DumpWriter(d, msg, key, kvs...)
}

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

func (l *Logger) Write(p []byte) (int, error) {
	if l == nil || l.Writer == nil {
		return len(p), nil
	}

	defer l.Unlock()
	l.Lock()

	return l.Writer.Write(p)
}

func (l *Logger) OK() bool { return l != nil }
func (s Span) OK() bool    { return s.Logger != nil }

func LoggerSetTimeNow(l *Logger, now func() time.Time, nano func() int64) {
	l.now = now
	l.nano = nano
}

func LoggerSetCallers(l *Logger, skip int, callers func(skip int, pc []uintptr) int) {
	l.callers = *(*func(int, *loc.PC, int, int) int)(unsafe.Pointer(&callers))
	l.callersSkip = skip + 1
	/*
		l.callers = func(skip int, pc *loc.PC, len, cap int) int {
			return callers(skip+2, *(*[]uintptr)(unsafe.Pointer(&struct {
				Ptr *loc.PC
				Len int
				Cap int
			}{
				Ptr: pc,
				Len: len,
				Cap: cap,
			})))
		}
	*/
}
