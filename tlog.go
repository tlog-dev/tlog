package tlog

import (
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/nikandfor/tlog/core"
	"github.com/nikandfor/tlog/loc"
	"github.com/nikandfor/tlog/low"
	"github.com/nikandfor/tlog/wire"
	"github.com/nikandfor/tlog/writer"
)

type (
	ID     = core.ID
	Type   = core.Type
	Level  = core.Level
	Labels = core.Labels

	Logger struct {
		wire.Encoder

		mu sync.Mutex

		tags []wire.Tag
		b    []byte

		filter *filter // accessed by atomic operations

		NoTime   bool
		NoCaller bool

		NewID func() ID
	}

	Span struct {
		Logger    *Logger
		ID        ID
		StartedAt time.Time
	}

	Option func(l *Logger)
)

// for you not to import os if you don't want.
var (
	Stderr = os.Stderr
	Stdout = os.Stdout
)

// Log Levels
const (
	Info = iota
	Warn
	Error
	Fatal

	Debug = -1
)

// Metric types
const (
	Counter   = "counter"
	Gauge     = "gauge"
	Summary   = "summary"
	Histogram = "histogram"
)

var ( // now
	unixnow = low.UnixNano
	now     = time.Now
)

var DefaultLogger = New(writer.NewConsole(os.Stderr, writer.LstdFlags), WithNoCaller)

func newspan(l *Logger, par ID, d int, args []interface{}) (s Span) {
	if l == nil {
		return
	}

	if !l.NoTime {
		s.StartedAt = now()
	}

	s.Logger = l
	s.ID = l.NewID()

	defer l.mu.Unlock()
	l.mu.Lock()

	defer func() {
		// NOTE: since we hacked compiler and made all arguments not escaping
		// we must zero all possible pointers to stack

		for i := range l.tags {
			l.tags[i].V = nil
		}

		l.tags = l.tags[:0]
	}()

	l.tags = wire.AppendTagVal(l.tags, wire.Span, s.ID)

	if !l.NoTime {
		l.tags = wire.AppendTagVal(l.tags, wire.Time, s.StartedAt.UnixNano())
	}
	if !l.NoCaller {
		l.tags = wire.AppendTagVal(l.tags, wire.Location, loc.Caller(d+2))
	}

	l.tags = wire.AppendTagVal(l.tags, wire.Type, wire.Start)

	if par != (ID{}) {
		l.tags = wire.AppendTagVal(l.tags, wire.Parent, par)
	}

	if len(args) != 0 {
		if name, ok := args[0].(string); ok {
			if name != "" {
				l.tags = wire.AppendTagVal(l.tags, wire.Name, name)
			}

			args = args[1:]
		}
	}

	wire.Event(&l.Encoder, l.tags, args)

	return
}

func newprint(l *Logger, id ID, d int16, lv Level, msg string, args []interface{}, kvs []interface{}) {
	if l == nil {
		return
	}

	defer l.mu.Unlock()
	l.mu.Lock()

	defer func() {
		// NOTE: since we hacked compiler and made all arguments not escaping
		// we must zero all possible pointers to stack

		for i := range l.tags {
			l.tags[i].V = nil
		}

		l.tags = l.tags[:0]
	}()

	if id != (ID{}) {
		l.tags = wire.AppendTagVal(l.tags, wire.Span, id)
	}

	if !l.NoTime {
		l.tags = wire.AppendTagVal(l.tags, wire.Time, unixnow())
	}
	if !l.NoCaller {
		l.tags = wire.AppendTagVal(l.tags, wire.Location, loc.Caller(int(d)+2))
	}

	if len(args) != 0 {
		l.tags = wire.AppendTagVal(l.tags, wire.Message, wire.Format{Fmt: msg, Args: args})
	} else {
		l.tags = wire.AppendTagVal(l.tags, wire.Message, msg)
	}

	wire.Event(&l.Encoder, l.tags, kvs)
}

func observe(l *Logger, id ID, name string, v interface{}, kvs []interface{}) {
	if l == nil {
		return
	}
	if v == nil {
		panic("nil value")
	}

	defer l.mu.Unlock()
	l.mu.Lock()

	defer func() {
		// NOTE: since we hacked compiler and made all arguments not escaping
		// we must zero all possible pointers to stack

		for i := range l.tags {
			l.tags[i].V = nil
		}

		l.tags = l.tags[:0]
	}()

	if id != (ID{}) {
		l.tags = wire.AppendTagVal(l.tags, wire.Span, id)
	}

	l.tags = wire.AppendTagVal(l.tags, wire.Name, name)
	l.tags = wire.AppendTagVal(l.tags, wire.Value, v)

	wire.Event(&l.Encoder, l.tags, kvs)
}

func New(w io.Writer, ops ...Option) *Logger {
	l := &Logger{
		NewID: core.MathRandID,
	}

	l.Writer = w

	for _, o := range ops {
		o(l)
	}

	return l
}

func (l *Logger) Event(tags []wire.Tag, kvs []interface{}) {
	if l == nil {
		return
	}

	defer l.mu.Unlock()
	l.mu.Lock()

	wire.Event(&l.Encoder, tags, kvs)
}

func (s Span) Event(tags []wire.Tag, kvs []interface{}) {
	l := s.Logger
	if l == nil {
		return
	}

	defer l.mu.Unlock()
	l.mu.Lock()

	defer func() {
		// NOTE: since we hacked compiler and made all arguments not escaping
		// we must zero all possible pointers to stack

		for i := range l.tags {
			l.tags[i].V = nil
		}

		l.tags = l.tags[:0]
	}()

	l.tags = wire.AppendTagVal(l.tags[:0], wire.Span, s.ID)
	l.tags = append(l.tags, tags...)

	wire.Event(&l.Encoder, l.tags, kvs)
}

func SetLabels(ls Labels) {
	DefaultLogger.SetLabels(ls)
}

func (l *Logger) SetLabels(ls Labels) {
	defer l.mu.Unlock()
	l.mu.Lock()

	wire.Event(&l.Encoder, []wire.Tag{{T: wire.Labels, V: ls}}, nil)
}

func Start(args ...interface{}) Span {
	return newspan(DefaultLogger, ID{}, 0, args)
}

func Spawn(par ID, args ...interface{}) Span {
	return newspan(DefaultLogger, par, 0, args)
}

func (l *Logger) Start(args ...interface{}) Span {
	return newspan(l, ID{}, 0, args)
}

func (l *Logger) Spawn(par ID, args ...interface{}) Span {
	return newspan(l, par, 0, args)
}

func (s Span) Finish(args ...interface{}) {
	l := s.Logger
	if l == nil {
		return
	}

	var d time.Duration
	if s.StartedAt != (time.Time{}) {
		d = now().Sub(s.StartedAt)
	}

	defer l.mu.Unlock()
	l.mu.Lock()

	defer func() {
		// NOTE: since we hacked compiler and made all arguments not escaping
		// we must zero all possible pointers to stack

		for i := range l.tags {
			l.tags[i].V = nil
		}

		l.tags = l.tags[:0]
	}()

	l.tags = wire.AppendTagVal(l.tags, wire.Span, s.ID)

	l.tags = wire.AppendTagVal(l.tags, wire.Type, wire.Finish)

	if d != 0 {
		l.tags = wire.AppendTagVal(l.tags, wire.SpanElapsed, d)
	}

	wire.Event(&l.Encoder, l.tags, nil)
}

func Printf(f string, args ...interface{}) {
	newprint(DefaultLogger, ID{}, 0, 0, f, args, nil)
}

func Printw(msg string, kvs ...interface{}) {
	newprint(DefaultLogger, ID{}, 0, 0, msg, nil, kvs)
}

func (l *Logger) Printf(f string, args ...interface{}) {
	newprint(l, ID{}, 0, 0, f, args, nil)
}

func (l *Logger) Printw(msg string, kvs ...interface{}) {
	newprint(l, ID{}, 0, 0, msg, nil, kvs)
}

func (s Span) Printf(f string, args ...interface{}) {
	newprint(s.Logger, s.ID, 0, 0, f, args, nil)
}

func (s Span) Printw(msg string, kvs ...interface{}) {
	newprint(s.Logger, s.ID, 0, 0, msg, nil, kvs)
}

// V checks if topic tp is enabled and returns default Logger or nil.
//
// It's OK to use nil Logger, it won't crash and won't emit any events to the Writer.
//
// Multiple comma separated topics could be provided. Logger will be non-nil if at least one of these topics is enabled.
//
// Usecases:
//     tlog.V("write").Printf("%d bytes written to address %v", n, addr)
//
//     if l := tlog.V("detailed"); l != nil {
//         c := 1 + 2 // do complex computations here
//         l.Printf("use result: %d")
//     }
func V(tp string) *Logger {
	if !DefaultLogger.ifv(tp) {
		return nil
	}

	return DefaultLogger
}

// If does the same checks as V but only returns bool.
func If(tp string) bool {
	return DefaultLogger.ifv(tp)
}

func (l *Logger) ifv(tp string) (ok bool) {
	if l == nil {
		return false
	}

	f := (*filter)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&l.filter))))
	if f == nil {
		return false
	}

	return f.match(tp, loc.Caller(2))
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

// If checks if some of topics enabled.
func (l *Logger) If(tp string) bool {
	return l.ifv(tp)
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

// SetFilter sets filter to use in V.
//
// Filter is a comma separated chain of rules.
// Each rule is applied to result of previous rule and adds or removes some locations.
// Rule started with '!' excludes matching locations.
//
// Each rule is one of: topic (some word you used in V argument)
//     error
//     networking
//     send
//     encryption
//
// location (directory, file, function) or
//     path/to/file.go
//     short_file.go
//     path/to/package - subpackages doesn't math
//     root/* - root package and all subpackages
//     github.com/nikandfor/tlog.Function
//     tlog.(*Type).Method
//     tlog.Type - all methods of type Type
//
// topics in location
//     tlog.Span=timing
//     p2p/conn.go=read+write - multiple topics in location are separated by '+'
//
// Example
//     module,!module/file.go,funcInFile
//
// SetFilter can be called simultaneously with V.
func SetFilter(f string) {
	DefaultLogger.SetFilter(f)
}

// Filter returns current verbosity filter of DefaultLogger.
func Filter() string {
	return DefaultLogger.Filter()
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

func Observe(name string, v float64, kvs ...interface{}) {
	observe(DefaultLogger, ID{}, name, v, kvs)
}

func (l *Logger) Observe(name string, v float64, kvs ...interface{}) {
	observe(l, ID{}, name, v, kvs)
}

func (s Span) Observe(name string, v float64, kvs ...interface{}) {
	observe(s.Logger, s.ID, name, v, kvs)
}

func RegisterMetric(name, typ, help string, kvs ...interface{}) {
	DefaultLogger.RegisterMetric(name, typ, help, kvs...)
}

func (l *Logger) RegisterMetric(name, typ, help string, kvs ...interface{}) {
	q := []interface{}{"type", typ, "help", help}
	q = append(q, kvs...)

	l.Event([]wire.Tag{{T: wire.Type, V: 'm'}, {T: wire.Name, V: name}}, q)
}
