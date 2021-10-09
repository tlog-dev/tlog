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
	"github.com/nikandfor/tlog/wire"
)

type (
	Logger struct {
		io.Writer

		NewID func() ID // must be threadsafe

		NoTime   bool
		NoCaller bool

		now  func() time.Time
		nano func() int64

		filter *filter // accessed by atomic operations

		wire.Encoder

		sync.Mutex

		b  []byte
		ls []byte
	}

	Span struct {
		Logger    *Logger
		ID        ID
		StartedAt time.Time
	}

	Message string

	EventType rune

	LogLevel int

	Hex int64

	FormatNext string

	Timestamp int64

	RawMessage []byte

	Format struct {
		Fmt  string
		Args []interface{}
	}

	// for log.SetOutput(l) // stdlib.
	writeWrapper struct {
		Span

		d int
	}
)

// for you not to import os
var (
	Stdin  = os.Stdin
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
	KeyTime      = "t"
	KeySpan      = "s"
	KeyParent    = "p"
	KeyMessage   = "m"
	KeyElapsed   = "e"
	KeyCaller    = "c"
	KeyLabels    = "L"
	KeyEventType = "T"
	KeyLogLevel  = "l"
)

// Metric types
const (
	MetricGauge   = "gauge"
	MetricCounter = "counter"
	MetricSummary = "summary"
)

// Event Types
const (
	EventLabels     = EventType('L')
	EventSpanStart  = EventType('s')
	EventSpanFinish = EventType('f')
	EventValue      = EventType('v')
	EventMetricDesc = EventType('m')
)

var DefaultLogger = New(NewConsoleWriter(Stderr, LstdFlags))

func New(w io.Writer) *Logger {
	l := &Logger{
		Writer: w,
		NewID:  MathRandID,
		now:    time.Now,
		nano:   low.UnixNano,
	}

	return l
}

func newmessage(l *Logger, id ID, d int, msg interface{}, kvs []interface{}) {
	if l == nil {
		return
	}

	defer l.Unlock()
	l.Lock()

	l.b = l.Encoder.AppendTag(l.b[:0], wire.Map, -1)

	if id != (ID{}) {
		l.b = l.Encoder.AppendString(l.b, wire.String, KeySpan)
		l.b = id.TlogAppend(&l.Encoder, l.b)
	}

	if !l.NoTime {
		l.b = l.Encoder.AppendString(l.b, wire.String, KeyTime)
		l.b = l.Encoder.AppendTimestamp(l.b, l.nano())
	}

	if !l.NoCaller && d >= 0 {
		var c loc.PC
		caller1(2+d, &c, 1, 1)

		l.b = l.Encoder.AppendString(l.b, wire.String, KeyCaller)
		l.b = l.Encoder.AppendPC(l.b, c)
	}

	if !low.IsNil(msg) {
		l.b = l.Encoder.AppendString(l.b, wire.String, KeyMessage)
		//l.b = append(l.b, wire.Semantic|WireMessage)
		l.b = l.Encoder.AppendTag(l.b, wire.Semantic, WireMessage)

		switch msg := msg.(type) {
		case string:
			l.b = l.Encoder.AppendString(l.b, wire.String, msg)
		case Format:
			if len(msg.Args) == 0 {
				l.b = l.Encoder.AppendString(l.b, wire.String, msg.Fmt)
				break
			}

			l.b = l.Encoder.AppendFormat(l.b, msg.Fmt, msg.Args...)
		case Message:
			l.b = l.Encoder.AppendString(l.b, wire.String, string(msg))
		case []byte:
			l.b = l.Encoder.AppendString(l.b, wire.String, low.UnsafeBytesToString(msg))
		default:
			l.b = l.Encoder.AppendFormat(l.b, "%v", msg)
		}
	}

	l.b = AppendKVs(&l.Encoder, l.b, kvs)

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

	l.b = l.Encoder.AppendTag(l.b[:0], wire.Map, -1)

	{
		l.b = l.Encoder.AppendString(l.b, wire.String, KeySpan)
		l.b = s.ID.TlogAppend(&l.Encoder, l.b)
	}

	if !l.NoTime {
		l.b = l.Encoder.AppendString(l.b, wire.String, KeyTime)
		l.b = l.Encoder.AppendTimestamp(l.b, l.nano())
	}

	if !l.NoCaller && d >= 0 {
		var c [2]loc.PC
		caller1(2+d, &c[0], 2, 2)

		l.b = l.Encoder.AppendString(l.b, wire.String, KeyCaller)
		l.b = l.Encoder.AppendPCs(l.b, c[:])
	}

	{
		l.b = l.Encoder.AppendString(l.b, wire.String, KeyEventType)
		l.b = EventSpanStart.TlogAppend(&l.Encoder, l.b)
	}

	if par != (ID{}) {
		l.b = l.Encoder.AppendString(l.b, wire.String, KeyParent)
		l.b = par.TlogAppend(&l.Encoder, l.b)
	}

	if n != "" {
		l.b = l.Encoder.AppendString(l.b, wire.String, KeyMessage)
		l.b = l.Encoder.AppendTag(l.b, wire.Semantic, WireMessage)
		l.b = l.Encoder.AppendString(l.b, wire.String, n)
	}

	l.b = AppendKVs(&l.Encoder, l.b, kvs)

	l.b = append(l.b, l.ls...)

	l.b = l.Encoder.AppendBreak(l.b)

	_, _ = l.Writer.Write(l.b)

	return
}

func newvalue(l *Logger, id ID, name string, v interface{}, kvs []interface{}) {
	if l == nil {
		return
	}

	defer l.Unlock()
	l.Lock()

	l.b = l.Encoder.AppendTag(l.b[:0], wire.Map, -1)

	if id != (ID{}) {
		l.b = l.Encoder.AppendString(l.b, wire.String, KeySpan)
		l.b = id.TlogAppend(&l.Encoder, l.b)
	}

	if !l.NoTime {
		l.b = l.Encoder.AppendString(l.b, wire.String, KeyTime)
		l.b = l.Encoder.AppendTimestamp(l.b, l.nano())
	}

	{
		l.b = l.Encoder.AppendString(l.b, wire.String, KeyEventType)
		l.b = EventValue.TlogAppend(&l.Encoder, l.b)
	}

	{
		l.b = l.Encoder.AppendString(l.b, wire.String, name)
		l.b = l.Encoder.AppendValue(l.b, v)
	}

	l.b = AppendKVs(&l.Encoder, l.b, kvs)

	l.b = append(l.b, l.ls...)

	l.b = l.Encoder.AppendBreak(l.b)

	_, _ = l.Writer.Write(l.b)
}

func (s Span) Finish(kvs ...interface{}) {
	if s.Logger == nil {
		return
	}

	defer s.Logger.Unlock()
	s.Logger.Lock()

	s.Logger.b = s.Logger.Encoder.AppendTag(s.Logger.b[:0], wire.Map, -1)

	if s.ID != (ID{}) {
		s.Logger.b = s.Logger.Encoder.AppendString(s.Logger.b, wire.String, KeySpan)
		s.Logger.b = s.ID.TlogAppend(&s.Logger.Encoder, s.Logger.b)
	}

	var now time.Time

	if !s.Logger.NoTime {
		now = s.Logger.now()

		s.Logger.b = s.Logger.Encoder.AppendString(s.Logger.b, wire.String, KeyTime)
		s.Logger.b = s.Logger.Encoder.AppendTime(s.Logger.b, now)
	}

	{
		s.Logger.b = s.Logger.Encoder.AppendString(s.Logger.b, wire.String, KeyEventType)
		s.Logger.b = EventSpanFinish.TlogAppend(&s.Logger.Encoder, s.Logger.b)
	}

	if !s.Logger.NoTime {
		if s.StartedAt != (time.Time{}) && s.StartedAt.UnixNano() != 0 {
			s.Logger.b = s.Logger.Encoder.AppendString(s.Logger.b, wire.String, KeyElapsed)
			s.Logger.b = s.Logger.Encoder.AppendDuration(s.Logger.b, now.Sub(s.StartedAt))
		}
	}

	s.Logger.b = AppendKVs(&s.Logger.Encoder, s.Logger.b, kvs)

	s.Logger.b = append(s.Logger.b, s.Logger.ls...)

	s.Logger.b = s.Logger.Encoder.AppendBreak(s.Logger.b)

	_, _ = s.Logger.Writer.Write(s.Logger.b)
}

func (l *Logger) Event(kvs ...interface{}) (err error) {
	if l == nil {
		return nil
	}

	defer l.Unlock()
	l.Lock()

	l.b = l.Encoder.AppendMap(l.b[:0], -1)

	l.b = AppendKVs(&l.Encoder, l.b, kvs)

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
		s.Logger.b = s.Logger.Encoder.AppendString(s.Logger.b, wire.String, KeySpan)
		s.Logger.b = s.ID.TlogAppend(&s.Logger.Encoder, s.Logger.b)
	}

	s.Logger.b = AppendKVs(&s.Logger.Encoder, s.Logger.b, kvs)

	s.Logger.b = append(s.Logger.b, s.Logger.ls...)

	s.Logger.b = s.Logger.Encoder.AppendBreak(s.Logger.b)

	_, err = s.Logger.Writer.Write(s.Logger.b)

	return
}

func SetLabels(ls Labels) {
	DefaultLogger.SetLabels(ls)
}

func (l *Logger) SetLabels(ls Labels) {
	if l == nil {
		return
	}

	sz := 2
	if !l.NoTime {
		sz = 3
	}

	defer l.Unlock()
	l.Lock()

	l.b = l.Encoder.AppendTag(l.b[:0], wire.Map, int64(sz))

	if !l.NoTime {
		l.b = l.Encoder.AppendString(l.b, wire.String, KeyTime)
		l.b = l.Encoder.AppendTimestamp(l.b, l.nano())
	}

	l.b = l.Encoder.AppendString(l.b, wire.String, KeyEventType)
	l.b = EventLabels.TlogAppend(&l.Encoder, l.b)

	st := len(l.b)

	l.b = l.Encoder.AppendString(l.b, wire.String, KeyLabels)
	l.b = ls.TlogAppend(&l.Encoder, l.b)

	l.ls = append(l.ls[:0], l.b[st:]...)

	_, _ = l.Writer.Write(l.b)
}

//go:noinline
func Printf(f string, args ...interface{}) {
	newmessage(DefaultLogger, ID{}, 0, Format{Fmt: f, Args: args}, nil)
}

//go:noinline
func Printw(msg string, kvs ...interface{}) {
	newmessage(DefaultLogger, ID{}, 0, msg, kvs)
}

//go:noinline
func (l *Logger) Printf(f string, args ...interface{}) {
	newmessage(l, ID{}, 0, Format{Fmt: f, Args: args}, nil)
}

//go:noinline
func (l *Logger) Printw(msg string, kvs ...interface{}) {
	newmessage(l, ID{}, 0, msg, kvs)
}

//go:noinline
func (s Span) Printf(f string, args ...interface{}) {
	newmessage(s.Logger, s.ID, 0, Format{Fmt: f, Args: args}, nil)
}

//go:noinline
func (s Span) Printw(msg string, kvs ...interface{}) {
	newmessage(s.Logger, s.ID, 0, msg, kvs)
}

//go:noinline
func Start(n string, kvs ...interface{}) Span {
	return newspan(DefaultLogger, ID{}, 0, n, kvs)
}

//go:noinline
func Spawn(p ID, n string, kvs ...interface{}) Span {
	return newspan(DefaultLogger, p, 0, n, kvs)
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

//go:noinline
func (l *Logger) NewMessage(d int, id ID, msg interface{}, kvs ...interface{}) {
	newmessage(l, id, d, msg, kvs)
}

//go:noinline
func (s Span) NewMessage(d int, msg interface{}, kvs ...interface{}) {
	newmessage(s.Logger, s.ID, d, msg, kvs)
}

func (l *Logger) ifv(d int, tp string) (ok bool) {
	if l == nil {
		return false
	}

	f := (*filter)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&l.filter))))
	if f == nil {
		return false
	}

	var loc loc.PC
	caller1(2+d, &loc, 1, 1)

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
	if !DefaultLogger.ifv(0, tp) {
		return nil
	}

	return DefaultLogger
}

// If does the same checks as V but only returns bool.
func If(tp string) bool {
	return DefaultLogger.ifv(0, tp)
}

// V checks if one of topics in tp is enabled and returns default Logger or nil.
//
// It's OK to use nil Logger, it won't crash and won't emit any events to writer.
//
// Multiple comma separated topics could be provided. Logger will be non-nil if at least one of these topics is enabled.
func (l *Logger) V(tp string) *Logger {
	if !l.ifv(0, tp) {
		return nil
	}

	return l
}

// If checks if some of topics enabled.
func (l *Logger) If(tp string) bool {
	return l.ifv(0, tp)
}

func (l *Logger) IfDepth(d int, tp string) bool {
	return l.ifv(d, tp)
}

// V checks if one of topics in tp is enabled and returns the same Span or empty overwise.
//
// It is safe to call any methods on empty Span.
//
// Multiple comma separated topics could be provided. Span will be Valid if at least one of these topics is enabled.
func (s Span) V(tp string) Span {
	if !s.Logger.ifv(0, tp) {
		return Span{}
	}

	return s
}

// If does the same checks as V but only returns bool.
func (s Span) If(tp string) bool {
	return s.Logger.ifv(0, tp)
}

func (s Span) IfDepth(d int, tp string) bool {
	return s.Logger.ifv(d, tp)
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
	if l == nil {
		return
	}

	if name == "" {
		panic("empty name")
	}

	if typ == "" {
		panic("empty type")
	}

	defer l.Unlock()
	l.Lock()

	l.b = l.Encoder.AppendMap(l.b[:0], -1)

	l.b = l.Encoder.AppendString(l.b, wire.String, KeyEventType)
	l.b = EventMetricDesc.TlogAppend(&l.Encoder, l.b)

	l.b = l.Encoder.AppendString(l.b, wire.String, "name")
	l.b = l.Encoder.AppendString(l.b, wire.String, name)

	l.b = l.Encoder.AppendString(l.b, wire.String, "type")
	l.b = l.Encoder.AppendString(l.b, wire.String, typ)

	if help != "" {
		l.b = l.Encoder.AppendString(l.b, wire.String, "help")
		l.b = l.Encoder.AppendString(l.b, wire.String, help)
	}

	l.b = AppendKVs(&l.Encoder, l.b, kvs)

	l.b = append(l.b, l.ls...)

	l.b = l.Encoder.AppendBreak(l.b)

	_, _ = l.Writer.Write(l.b)
}

func TestSetTime(l *Logger, t func() time.Time, ts func() int64) {
	l.now = t
	l.nano = ts
}
