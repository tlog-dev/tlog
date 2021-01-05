package tlog

import (
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/nikandfor/loc"
	"github.com/nikandfor/tlog/low"
)

type (
	Logger struct {
		sync.Mutex

		Encoder

		NewID func() ID // must be threadsafe

		NoTime   bool
		NoCaller bool

		buf []interface{}

		filter *filter // accessed by atomic operations
	}

	Span struct {
		Logger    *Logger
		ID        ID
		StartedAt time.Time
	}

	// for log.SetOutput(l) // stdlib.
	writeWrapper struct {
		Span

		d int
	}
)

// for you not to import os
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

	Debug = -1
)

// Predefined keys
var (
	KeyTime      = "t"
	KeySpan      = "s"
	KeyParent    = "p"
	KeyMessage   = "m"
	KeyElapsed   = "e"
	KeyLocation  = "l"
	KeyLabels    = "L"
	KeyEventType = "T"
	KeyLogLevel  = "i"
)

// Metric types
const (
	MetricGauge   = "gauge"
	MetricCounter = "counter"
	MetricSummary = "summary"
)

var ( //time
	now  = time.Now
	nano = low.UnixNano
)

var DefaultLogger = New(NewConsoleWriter(os.Stderr, LstdFlags))

var zeroBuf = make([]interface{}, 30)

func New(w io.Writer) *Logger {
	l := &Logger{
		Encoder: Encoder{
			Writer: w,
		},
		NewID: MathRandID,
	}

	return l
}

func newmessage(l *Logger, id ID, d int, msg interface{}, kvs []interface{}) {
	if l == nil {
		return
	}

	var t Timestamp
	if !l.NoTime {
		t = Timestamp(nano())
	}

	var lc loc.PC
	if !l.NoCaller && d >= 0 {
		caller1(2+d, &lc, 1, 1)
	}

	defer l.Unlock()
	l.Lock()

	// TODO l.clearBuf

	if id != (ID{}) {
		l.appendBuf(KeySpan, id)
	}

	if !l.NoTime {
		l.appendBuf(KeyTime, t)
	}
	if !l.NoCaller {
		l.appendBuf(KeyLocation, lc)
	}

	if msg != nil && msg != Message("") {
		l.appendBuf(KeyMessage, msg)
	}

	_ = l.Encoder.Encode(l.buf, kvs)

	l.clearBuf()
}

func newspan(l *Logger, par ID, d int, n string, kvs []interface{}) (s Span) {
	if l == nil {
		return
	}

	s.Logger = l
	s.ID = l.NewID()
	s.StartedAt = now()

	var lc loc.PC
	if !l.NoCaller && d >= 0 {
		caller1(2+d, &lc, 1, 1)
	}

	defer l.Unlock()
	l.Lock()

	defer l.clearBuf()

	l.appendBuf(KeySpan, s.ID)

	if !l.NoTime {
		l.appendBuf(KeyTime, Timestamp(s.StartedAt.UnixNano()))
	}
	if !l.NoCaller {
		l.appendBuf(KeyLocation, lc)
	}

	l.appendBuf(KeyEventType, EventType("s"))

	if par != (ID{}) {
		l.appendBuf(KeyParent, par)
	}

	if n != "" {
		l.appendBuf(KeyMessage, Message(n))
	}

	_ = l.Encoder.Encode(l.buf, kvs)

	return
}

func newvalue(l *Logger, id ID, name string, v interface{}, kvs []interface{}) {
	if l == nil {
		return
	}

	var t Timestamp
	if !l.NoTime {
		t = Timestamp(nano())
	}

	defer l.Unlock()
	l.Lock()

	defer l.clearBuf()

	if id != (ID{}) {
		l.appendBuf(KeySpan, id)
	}

	if !l.NoTime {
		l.appendBuf(KeyTime, t)
	}

	l.appendBuf(KeyEventType, EventType("v"))

	l.appendBuf(name, v)

	_ = l.Encoder.Encode(l.buf, kvs)
}

func (s Span) Finish(kvs ...interface{}) {
	if s.Logger == nil {
		return
	}

	var el time.Duration
	if !s.Logger.NoTime {
		el = now().Sub(s.StartedAt)
	}

	defer s.Logger.Unlock()
	s.Logger.Lock()

	defer s.Logger.clearBuf()

	s.Logger.appendBuf(KeySpan, s.ID)
	s.Logger.appendBuf(KeyEventType, EventType("f"))

	if el != 0 {
		s.Logger.appendBuf(KeyElapsed, el)
	}

	_ = s.Logger.Encoder.Encode(s.Logger.buf, kvs)
}

func (l *Logger) Event2(kvs ...[]interface{}) error {
	if l == nil {
		return nil
	}

	defer l.Unlock()
	l.Lock()

	return l.Encoder.Encode(nil, kvs...)
}

func (s Span) Event2(kvs ...[]interface{}) error {
	if s.Logger == nil {
		return nil
	}

	defer s.Logger.Unlock()
	s.Logger.Lock()

	defer s.Logger.clearBuf()

	if s.ID != (ID{}) {
		s.Logger.appendBuf(KeySpan, s.ID)
	}

	return s.Logger.Encoder.Encode(s.Logger.buf, kvs...)
}

func (l *Logger) Event(kvs ...interface{}) error {
	if l == nil {
		return nil
	}

	defer l.Unlock()
	l.Lock()

	return l.Encoder.Encode(nil, kvs)
}

func (s Span) Event(kvs ...interface{}) error {
	if s.Logger == nil {
		return nil
	}

	defer s.Logger.Unlock()
	s.Logger.Lock()

	defer s.Logger.clearBuf()

	if s.ID != (ID{}) {
		s.Logger.appendBuf(KeySpan, s.ID)
	}

	return s.Logger.Encoder.Encode(s.Logger.buf, kvs)
}

func SetLabels(ls Labels) {
	DefaultLogger.SetLabels(ls)
}

func (l *Logger) SetLabels(ls Labels) {
	l.Event2([]interface{}{KeyLabels, ls})
}

//go:noinline
func Printf(f string, args ...interface{}) {
	newmessage(DefaultLogger, ID{}, 0, Format{Fmt: f, Args: args}, nil)
}

//go:noinline
func Printw(msg string, kvs ...interface{}) {
	newmessage(DefaultLogger, ID{}, 0, Message(msg), kvs)
}

//go:noinline
func PrintwDepth(d int, msg string, kvs ...interface{}) {
	newmessage(DefaultLogger, ID{}, d, Message(msg), kvs)
}

//go:noinline
func (l *Logger) Printf(f string, args ...interface{}) {
	newmessage(l, ID{}, 0, Format{Fmt: f, Args: args}, nil)
}

//go:noinline
func (l *Logger) Printw(msg string, kvs ...interface{}) {
	newmessage(l, ID{}, 0, Message(msg), kvs)
}

//go:noinline
func (l *Logger) PrintwDepth(d int, msg string, kvs ...interface{}) {
	newmessage(l, ID{}, d, Message(msg), kvs)
}

//go:noinline
func (s Span) Printf(f string, args ...interface{}) {
	newmessage(s.Logger, s.ID, 0, Format{Fmt: f, Args: args}, nil)
}

//go:noinline
func (s Span) Printw(msg string, kvs ...interface{}) {
	newmessage(s.Logger, s.ID, 0, Message(msg), kvs)
}

//go:noinline
func (s Span) PrintwDepth(d int, msg string, kvs ...interface{}) {
	newmessage(s.Logger, s.ID, d, Message(msg), kvs)
}

func Start(n string, kvs ...interface{}) Span {
	return newspan(DefaultLogger, ID{}, 0, n, kvs)
}

func (l *Logger) Start(n string, kvs ...interface{}) Span {
	return newspan(l, ID{}, 0, n, kvs)
}

func (l *Logger) Spawn(par ID, n string, kvs ...interface{}) Span {
	if par == (ID{}) {
		return Span{}
	}
	return newspan(l, par, 0, n, kvs)
}

func (s Span) Spawn(n string, kvs ...interface{}) Span {
	if s.ID == (ID{}) {
		return Span{}
	}
	return newspan(s.Logger, s.ID, 0, n, kvs)
}

func (l *Logger) SpawnOrStart(par ID, n string, kvs ...interface{}) Span {
	return newspan(l, par, 0, n, kvs)
}

func (s Span) SpawnOrStart(n string, kvs ...interface{}) Span {
	return newspan(s.Logger, s.ID, 0, n, kvs)
}

func (l *Logger) NewSpan(d int, par ID, name string, kvs ...interface{}) Span {
	return newspan(l, par, d, name, kvs)
}

func (l *Logger) ifv(tp string) (ok bool) {
	if l == nil {
		return false
	}

	f := (*filter)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&l.filter))))
	if f == nil {
		return false
	}

	var loc loc.PC
	caller1(2, &loc, 1, 1)

	return f.match(tp, loc)
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

// V checks if one of topics in tp is enabled and returns default Logger or nil.
//
// It's OK to use nil Logger, it won't crash and won't emit any events to writer.
//
// Multiple comma separated topics could be provided. Logger will be non-nil if at least one of these topics is enabled.
func (l *Logger) V(tp string) *Logger {
	if !l.ifv(tp) {
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
	newmessage(w.Logger, w.ID, w.d, low.UnsafeBytesToString(p), nil)

	return len(p), nil
}

func (l *Logger) Observe(name string, v interface{}, kvs ...interface{}) {
	newvalue(l, ID{}, name, v, kvs)
}

func (s Span) Observe(name string, v interface{}, kvs ...interface{}) {
	newvalue(s.Logger, s.ID, name, v, kvs)
}

func RegisterMetric(name, typ, help string, kvs ...interface{}) {
	DefaultLogger.RegisterMetric(name, typ, help, kvs...)
}

func (l *Logger) RegisterMetric(name, typ, help string, kvs ...interface{}) {
	l.Event2([]interface{}{
		KeyEventType, EventType("m"),
		KeyMessage, name,
		"type", typ,
		"help", help,
	}, kvs)
}

func (l *Logger) appendBuf(vals ...interface{}) {
	l.buf = append0(l.buf, vals...)
}

func append1(b []interface{}, v ...interface{}) []interface{} {
	return append(b, v...)
}

func (l *Logger) clearBuf() {
	for i := 0; i < len(l.buf); {
		i += copy(l.buf[i:], zeroBuf)
	}

	l.buf = l.buf[:0]
}

func TestSetTime(t func() time.Time, ts func() int64) {
	now = t
	nano = ts
}
