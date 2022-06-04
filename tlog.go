package tlog

import (
	"io"
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

		NewID func() ID // must be threadsafe

		AppendTimestamp func([]byte) []byte
		AppendCaller    func([]byte, int) []byte

		now func() time.Time

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

	// for like stdlib log.SetOutput(l)
	writeWrapper struct {
		Span

		d int
	}
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

var DefaultLogger = New(nil)

func Root() Span { return Span{Logger: DefaultLogger} }

func New(w io.Writer) *Logger {
	return &Logger{
		Writer:          w,
		NewID:           MathRandID,
		AppendTimestamp: AppendTimestamp,
		AppendCaller:    AppendCaller,
		now:             time.Now,
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

	l.b = l.AppendTimestamp(l.b)

	if d >= 0 {
		l.b = l.AppendCaller(l.b, 2+d)
	}

	if msg != nil {
		l.b = l.Encoder.AppendKey(l.b, KeyMessage)

		l.b = l.Encoder.AppendSemantic(l.b, WireMessage)

		switch msg := msg.(type) {
		case string:
			l.b = l.Encoder.AppendString(l.b, msg)
		case []byte:
			l.b = l.Encoder.AppendTagBytes(l.b, tlwire.String, msg)
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
	s.StartedAt = l.now()

	defer l.Unlock()
	l.Lock()

	l.b = l.Encoder.AppendMap(l.b[:0], -1)

	l.b = l.Encoder.AppendString(l.b, KeySpan)
	l.b = s.ID.TlogAppend(l.b)

	l.b = l.AppendTimestamp(l.b)

	if d >= 0 {
		l.b = l.AppendCaller(l.b, 2+d)
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

func SetLabels(kvs ...interface{}) {
	DefaultLogger.SetLabels(kvs...)
}

func Start(name string, kvs ...interface{}) Span {
	return newspan(DefaultLogger, ID{}, 0, name, kvs)
}

func Printw(msg string, kvs ...interface{}) {
	message(DefaultLogger, ID{}, 0, msg, kvs)
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

func (l *Logger) SetLabels(kvs ...interface{}) {
	if l == nil {
		return
	}

	defer l.Unlock()
	l.Lock()

	const tag = tlwire.Semantic | WireLabel

	l.ls = append(l.ls[:0], low.Spaces[:len(kvs)/2]...)

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

func (l *Logger) Event(kvs ...interface{}) (err error) {
	if l == nil {
		return nil
	}

	defer l.Unlock()
	l.Lock()

	l.b = l.Encoder.AppendMap(l.b[:0], -1)

	l.b = AppendKVs(l.b, kvs)

	l.b = append(l.b, l.ls...)

	l.b = l.Encoder.AppendBreak(l.b)

	_, err = l.Writer.Write(l.b)

	return
}

func (s Span) Event(kvs ...interface{}) (err error) {
	if s.Logger == nil {
		return nil
	}

	defer s.Logger.Unlock()
	s.Logger.Lock()

	s.Logger.b = s.Logger.Encoder.AppendMap(s.Logger.b[:0], -1)

	if s.ID != (ID{}) {
		s.Logger.b = s.Logger.Encoder.AppendString(s.Logger.b, KeySpan)
		s.Logger.b = s.ID.TlogAppend(s.Logger.b)
	}

	s.Logger.b = AppendKVs(s.Logger.b, kvs)

	s.Logger.b = append(s.Logger.b, s.Logger.ls...)

	s.Logger.b = s.Logger.Encoder.AppendBreak(s.Logger.b)

	_, err = s.Logger.Writer.Write(s.Logger.b)

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

func (l *Logger) Printw(msg string, kvs ...interface{}) {
	message(l, ID{}, 0, msg, kvs)
}

func (s Span) Printw(msg string, kvs ...interface{}) {
	message(s.Logger, s.ID, 0, msg, kvs)
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

func AppendTimestamp(b []byte) []byte {
	b = e.AppendKey(b, KeyTimestamp)
	return e.AppendTimestamp(b, htime.UnixNano())
}

func AppendCaller(b []byte, d int) []byte {
	var c loc.PC
	caller1(1+d, &c, 1, 1)

	b = e.AppendKey(b, KeyCaller)

	return e.AppendPC(b, c)
}
