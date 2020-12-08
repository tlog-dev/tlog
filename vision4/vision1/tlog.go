package tlog

import (
	"context"
	"io"
	"os"
	"sync/atomic"
	"time"
	"unsafe"
)

type (
	ID [16]byte

	Type  byte
	Level int8

	A struct {
		Name  string
		Value interface{}
	}

	D map[string]interface{}

	Logger struct {
		Writer

		Hooks []Hook

		filter *filter

		NewID func() ID
	}

	Writer interface {
		Write(e *Event) error
	}

	Span struct {
		*Logger
		ID ID
	}

	Hook func(ctx context.Context, ev *Event) error

	Event struct {
		Context context.Context
		Logger  *Logger
		Span    ID
		Time    time.Time
		PC      PC
		Type    Type
		Level   Level
		Attrs   []A
		b       bufWriter
		s       []string
		sorter  sorter
	}

	// ShortIDError is an ID parsing error.
	ShortIDError struct {
		N int
	}

	writeWrapper struct {
		Span

		d  int
		lv Level
	}
)

// Log levels.
const (
	Info = iota
	Warn
	Error
	Fatal

	Debug = -1
)

// Metric types.
const (
	MCounter   = "counter"
	MGauge     = "gauge"
	MSummary   = "summary"
	MUntyped   = "untyped"
	MHistogram = "histogram"
	Mempty     = ""
)

// Meta types.
const (
	MetaMetricDescription = "metric_desc"
)

// for you not to import os if you don't want.
var (
	Stderr = os.Stderr
	Stdout = os.Stdout
)

var now = time.Now

var DefaultLogger = New(&Logger{
	Writer: NewConsoleWriter(os.Stderr, LstdFlags),
	Hooks: []Hook{
		AddNow,
	},
	NewID: MathRandID,
})

func New(ws ...Writer) (l *Logger) {
	l = &Logger{
		Hooks: []Hook{
			AddNow,
			AddCaller,
		},
		NewID: MathRandID,
	}

	switch len(ws) {
	case 0:
		l.Writer = Discard
	case 1:
		l.Writer = ws[0]
	default:
		l.Writer = NewTeeWriter(ws...)
	}

	return l
}

func (l *Logger) Ev(ctx context.Context, lv Level) *Event {
	return Ev(ctx, l, ID{}, lv)
}

func (l *Logger) Printf(f string, args ...interface{}) {
	Ev(nil, l, ID{}, 0).Message(f, args...)
}

func (l *Logger) Printw(f string, kv D) {
	Ev(nil, l, ID{}, 0).Dict(kv).Message(f)
}

func (l *Logger) Errorf(f string, args ...interface{}) {
	Ev(nil, l, ID{}, Error).Message(f, args...)
}

func (l *Logger) Start() Span {
	if l == nil {
		return Span{}
	}

	return Ev(nil, l, l.NewID(), 0).Start()
}

func (l *Logger) Spawn(par ID) Span {
	if l == nil {
		return Span{}
	}

	return Ev(nil, l, l.NewID(), 0).Spawn(par)
}

func (l *Logger) SetLabels(ls Labels) {
	Ev(nil, l, ID{}, 0).Labels(ls).Write()
}

func (l *Logger) Observe(name string, v float64, ls Labels) {
	ev := Ev(nil, l, ID{}, 0).Tp('v').Str("n", name).Flt("v", v)

	if ls != nil {
		ev.Any("L", ls)
	}

	ev.Write()
}

func (l *Logger) RegisterMetric(name, typ, help string, ls Labels) {
	Ev(nil, l, ID{}, 0).Any("M", []A{
		{Name: "T", Value: MetaMetricDescription},
		{Name: "n", Value: name},
		{Name: "t", Value: typ},
		{Name: "h", Value: help},
		{Name: "L", Value: ls},
	}).Write()
}

func (l *Logger) ifv(tp string) (ok bool) {
	if l == nil {
		return false
	}

	f := (*filter)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&l.filter))))
	if f == nil {
		return false
	}

	return f.match(tp)
}

// If checks if some of topics enabled.
func (l *Logger) If(tp string) bool {
	return l.ifv(tp)
}

// V checks if one of topics in tp is enabled and returns default Logger or nil.
//
// It's OK to use nil Logger, it won't crash and won't emit any events to writer.
//
// Multiple comma separated topics could be provided. Logger will be non-nil if at least one of these topics is enabled.
func (l *Logger) V(tp string) *Logger {
	if l == nil || !l.ifv(tp) {
		return nil
	}

	return l
}

// SetFilter sets filter to use in V.
//
// See package.SetFilter description for details.
func (l *Logger) SetFilter(filters string) {
	if l == nil {
		return
	}

	f := newFilter(filters)

	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&l.filter)), unsafe.Pointer(f))
}

// Filter returns current verbosity filter value.
//
// See package.SetFilter description for details.
func (l *Logger) Filter() string {
	if l == nil {
		return ""
	}

	f := (*filter)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&l.filter))))
	if f == nil {
		return ""
	}

	return f.f
}

func (l *Logger) IOWriter(d int, lv Level) io.Writer {
	return writeWrapper{
		Span: Span{
			Logger: l,
		},
		d:  d,
		lv: lv,
	}
}

func (s Span) Ev(ctx context.Context, lv Level) *Event {
	return Ev(ctx, s.Logger, s.ID, lv)
}

func (s Span) Finish() {
	Ev(nil, s.Logger, s.ID, 0).Finish()
}

func (s Span) Spawn() Span {
	if s.Logger == nil {
		return Span{}
	}

	return Ev(nil, s.Logger, s.Logger.NewID(), 0).Spawn(s.ID)
}

func (s Span) Printf(f string, args ...interface{}) {
	Ev(nil, s.Logger, s.ID, 0).Message(f, args...)
}

func (s Span) Printw(f string, kv D) {
	Ev(nil, s.Logger, s.ID, 0).Dict(kv).Message(f)
}

func (s Span) Errorf(f string, args ...interface{}) {
	Ev(nil, s.Logger, s.ID, Error).Message(f, args...)
}

func (s Span) SetLabels(ls Labels) {
	Ev(nil, s.Logger, s.ID, 0).Labels(ls).Write()
}

func (s Span) Observe(name string, v float64, ls Labels) {
	ev := Ev(nil, s.Logger, s.ID, 0).Tp('v').Str("n", name).Flt("v", v)

	if ls != nil {
		ev.Any("L", ls)
	}

	ev.Write()
}

// V checks if one of topics in tp is enabled and returns the same Span or empty overwise.
//
// It is safe to call any methods on empty Span.
//
// Multiple comma separated topics could be provided. Span will be Valid if at least one of these topics is enabled.
func (s Span) V(tp string) Span {
	if !s.Logger.ifv(tp) {
		return Span{}
	}

	return s
}

// If does the same checks as V but only returns bool.
func (s Span) If(tp string) bool {
	return s.Logger.ifv(tp)
}

// WriteWrapper returns an io.Writer interface implementation.
func (s Span) IOWriter(d int, lv Level) io.Writer {
	return writeWrapper{
		Span: s,
		d:    d,
		lv:   lv,
	}
}

func (w writeWrapper) Write(p []byte) (_ int, err error) {
	if w.Logger == nil {
		return len(p), nil
	}

	ev := w.Span.Ev(nil, w.lv)

	if w.d > 0 {
		ev.Caller(1 + w.d)
	}

	ev.Str("m", bytesToString(p))

	err = ev.Write()
	if err != nil {
		return
	}

	return len(p), nil
}
