package tlog

import (
	"io"
	"os"
	"sync"
	"time"

	"github.com/nikandfor/hacked/htime"
	"github.com/nikandfor/loc"
	"github.com/nikandfor/tlog/low"
	"github.com/nikandfor/tlog/tlwire"
)

type (
	Logger struct {
		io.Writer

		tlwire.Encoder

		NewID func() ID `deep:"compare=pointer"` // must be threadsafe

		now  func() time.Time `deep:"compare=pointer"`
		nano func() int64     `deep:"compare=pointer"`

		filter *filter // atomic access

		sync.Mutex

		b  []byte
		ls []byte
	}

	Span struct {
		*Logger
		ID        ID
		StartedAt time.Time
	}

	LogLevel int

	EventKind rune

	// for like stdlib log.SetOutput(l)
	writeWrapper struct {
		Span

		d int
	}
)

var (
	Stdout = os.Stdout
	Stderr = os.Stderr
)

// Log levels
const (
	Info LogLevel = iota
	Warn
	Error
	Fatal

	Debug LogLevel = -1
)

// Predefined keys
var (
	KeySpan      = "_s"
	KeyParent    = "_p"
	KeyTimestamp = "_t"
	KeyElapsed   = "_e"
	KeyCaller    = "_c"
	KeyMessage   = "_m"
	KeyEventKind = "_k"
	KeyLogLevel  = "_l"
)

// Event kinds
const (
	EventLabels     EventKind = 'l'
	EventSpanStart  EventKind = 's'
	EventSpanFinish EventKind = 'f'
	EventValue      EventKind = 'v'
	EventMetricDesc EventKind = 'm'
)

var DefaultLogger = New(NewConsoleWriter(os.Stderr, LdetFlags))

func Root() Span { return Span{Logger: DefaultLogger} }

func (l *Logger) Root() Span { return Span{Logger: l} }

func New(w io.Writer) *Logger {
	return &Logger{
		Writer: w,
		NewID:  MathRandID,
		now:    time.Now,
		nano:   htime.UnixNano,
	}
}

func message(l *Logger, id ID, d int, msg interface{}, kvs []interface{}) {
	if l == nil {
		return
	}

	defer l.Unlock()
	l.Lock()

	l.b = l.Encoder.AppendMap(l.b[:0], -1)

	if id != (ID{}) {
		l.b = l.Encoder.AppendString(l.b, KeySpan)
		l.b = id.TlogAppend(l.b)
	}

	if l.nano != nil {
		now := l.nano()

		l.b = l.Encoder.AppendString(l.b, KeyTimestamp)
		l.b = l.Encoder.AppendTimestamp(l.b, now)
	}

	if d >= 0 {
		var c loc.PC
		caller1(2+d, &c, 1, 1)

		l.b = e.AppendKey(l.b, KeyCaller)
		l.b = e.AppendCaller(l.b, c)
	}

	if msg != nil {
		l.b = l.Encoder.AppendKey(l.b, KeyMessage)

		l.b = l.Encoder.AppendSemantic(l.b, WireMessage)

		switch msg := msg.(type) {
		case string:
			l.b = l.Encoder.AppendString(l.b, msg)
		case []byte:
			l.b = l.Encoder.AppendTagBytes(l.b, tlwire.String, msg)
		case format:
			l.b = l.Encoder.AppendFormat(l.b, msg.Fmt, msg.Args...)
		default:
			l.b = l.Encoder.AppendFormat(l.b, "%v", msg)
		}
	}

	l.b = AppendKVs(l.b, kvs)

	l.b = append(l.b, l.ls...)

	l.b = l.Encoder.AppendBreak(l.b)

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

	defer l.Unlock()
	l.Lock()

	l.b = l.Encoder.AppendMap(l.b[:0], -1)

	l.b = l.Encoder.AppendString(l.b, KeySpan)
	l.b = s.ID.TlogAppend(l.b)

	if l.now != nil {
		l.b = l.Encoder.AppendString(l.b, KeyTimestamp)
		l.b = l.Encoder.AppendTimestamp(l.b, s.StartedAt.UnixNano())
	}

	if d >= 0 {
		var c loc.PC
		caller1(2+d, &c, 1, 1)

		l.b = e.AppendKey(l.b, KeyCaller)
		l.b = e.AppendCaller(l.b, c)
	}

	l.b = l.Encoder.AppendString(l.b, KeyEventKind)
	l.b = EventSpanStart.TlogAppend(l.b)

	if par != (ID{}) {
		l.b = l.Encoder.AppendString(l.b, KeyParent)
		l.b = par.TlogAppend(l.b)
	}

	if n != "" {
		l.b = l.Encoder.AppendString(l.b, KeyMessage)
		l.b = l.Encoder.AppendSemantic(l.b, WireMessage)
		l.b = l.Encoder.AppendString(l.b, n)
	}

	l.b = AppendKVs(l.b, kvs)

	l.b = append(l.b, l.ls...)

	l.b = l.Encoder.AppendBreak(l.b)

	_, _ = l.Writer.Write(l.b)

	return
}

func (s Span) Finish(kvs ...interface{}) {
	if s.Logger == nil {
		return
	}

	l := s.Logger

	defer l.Unlock()
	l.Lock()

	l.b = l.Encoder.AppendTag(l.b[:0], tlwire.Map, -1)

	if s.ID != (ID{}) {
		l.b = l.Encoder.AppendString(l.b, KeySpan)
		l.b = s.ID.TlogAppend(l.b)
	}

	var now time.Time
	if l.now != nil {
		now = l.now()

		l.b = l.Encoder.AppendString(l.b, KeyTimestamp)
		l.b = l.Encoder.AppendTimestamp(l.b, now.UnixNano())
	}

	l.b = l.Encoder.AppendString(l.b, KeyEventKind)
	l.b = EventSpanFinish.TlogAppend(l.b)

	if l.now != nil {
		l.b = l.Encoder.AppendString(l.b, KeyElapsed)
		l.b = l.Encoder.AppendDuration(l.b, now.Sub(s.StartedAt))
	}

	l.b = AppendKVs(l.b, kvs)

	l.b = append(l.b, l.ls...)

	l.b = l.Encoder.AppendBreak(l.b)

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

	const tag = tlwire.Semantic | WireLabel

	l.ls = append(l.ls[:0], low.Spaces[:len(kvs)/2+1]...)

	w, r := 0, len(l.ls)

	l.ls = AppendKVs(l.ls, kvs)

	for r < len(l.ls) {
		end := d.Skip(l.ls, r)

		w += copy(l.ls[w:], l.ls[r:end])
		r = end

		end = d.Skip(l.ls, r)

		if l.ls[r] != tag {
			l.ls[w] = tag
			w++
		}

		w += copy(l.ls[w:], l.ls[r:end])
		r = end
	}

	l.ls = l.ls[:w]
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

	l.b = AppendKVs(l.b, kvs)

	l.b = append(l.b, l.ls...)

	l.b = l.AppendBreak(l.b)

	_, err = l.Writer.Write(l.b)

	return
}

func (s Span) Event(kvs ...interface{}) (err error) {
	if s.Logger == nil {
		return nil
	}

	defer s.Unlock()
	s.Lock()

	s.b = s.AppendMap(s.b[:0], -1)

	if s.ID != (ID{}) {
		s.b = s.AppendString(s.b, KeySpan)
		s.b = s.ID.TlogAppend(s.b)
	}

	s.b = AppendKVs(s.b, kvs)

	s.b = append(s.b, s.ls...)

	s.b = s.AppendBreak(s.b)

	_, err = s.Writer.Write(s.b)

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

func (w writeWrapper) Write(p []byte) (int, error) {
	message(w.Logger, w.ID, w.d, p, nil)

	return len(p), nil
}

func (l *Logger) Write(p []byte) (int, error) {
	if l == nil || l.Writer == nil {
		return len(p), nil
	}

	return l.Writer.Write(p)
}

func LoggerSetTimeNow(l *Logger, now func() time.Time, nano func() int64) {
	l.now = now
	l.nano = nano
}
