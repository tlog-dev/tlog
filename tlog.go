package tlog

import (
	"io"
	"os"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/nikandfor/loc"
	"github.com/nikandfor/tlog/low"
)

type (
	Logger struct {
		Encoder

		NewID func() ID // must be threadsafe

		NoTime   bool
		NoCaller bool

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

var DefaultLogger = New(NewConsoleWriter(Stderr, LstdFlags))

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

	var hdr [8]interface{}
	i := 0

	if id != (ID{}) {
		hdr[i] = KeySpan
		i++

		hdr[i] = id
		i++
	}

	if !l.NoTime {
		hdr[i] = KeyTime
		i++

		hdr[i] = Timestamp(nano())
		i++
	}

	if !l.NoCaller && d >= 0 {
		var lc loc.PC

		caller1(2+d, &lc, 1, 1)

		hdr[i] = KeyLocation
		i++

		hdr[i] = lc
		i++
	}

	if s, ok := msg.(string); ok {
		msg = Message(s)
	}

	if msg != nil && msg != Message("") {
		hdr[i] = KeyMessage
		i++

		hdr[i] = msg
		i++
	}

	_ = l.Encoder.Encode(hdr[:i], kvs)
}

func newspan(l *Logger, par ID, d int, n string, kvs []interface{}) (s Span) {
	if l == nil {
		return
	}

	s.Logger = l
	s.ID = l.NewID()
	s.StartedAt = now()

	var hdr [12]interface{}
	i := 0

	{
		hdr[i] = KeySpan
		i++

		hdr[i] = s.ID
		i++
	}

	if !l.NoTime {
		hdr[i] = KeyTime
		i++

		hdr[i] = Timestamp(nano())
		i++
	}

	if !l.NoCaller && d >= 0 {
		var lc loc.PC

		caller1(2+d, &lc, 1, 1)

		hdr[i] = KeyLocation
		i++

		hdr[i] = lc
		i++
	}

	{
		hdr[i] = KeyEventType
		i++

		hdr[i] = EventType("s")
		i++
	}

	if par != (ID{}) {
		hdr[i] = KeyParent
		i++

		hdr[i] = par
		i++
	}

	if n != "" {
		hdr[i] = KeyMessage
		i++

		hdr[i] = Message(n)
		i++
	}

	_ = l.Encoder.Encode(hdr[:i], kvs)

	return
}

func newvalue(l *Logger, id ID, name string, v interface{}, kvs []interface{}) {
	if l == nil {
		return
	}

	var hdr [8]interface{}
	i := 0

	if id != (ID{}) {
		hdr[i] = KeySpan
		i++

		hdr[i] = id
		i++
	}

	if !l.NoTime {
		hdr[i] = KeyTime
		i++

		hdr[i] = Timestamp(nano())
		i++
	}

	{
		hdr[i] = KeyEventType
		i++

		hdr[i] = EventType("v")
		i++
	}

	{
		hdr[i] = name
		i++

		hdr[i] = v
		i++
	}

	_ = l.Encoder.Encode(hdr[:i], kvs)
}

func (s Span) Finish(kvs ...interface{}) {
	if s.Logger == nil {
		return
	}

	var hdr [8]interface{}
	i := 0

	if s.ID != (ID{}) {
		hdr[i] = KeySpan
		i++

		hdr[i] = s.ID
		i++
	}

	if !s.Logger.NoTime {
		now := now()

		hdr[i] = KeyTime
		i++

		hdr[i] = Timestamp(now.UnixNano())
		i++

		if s.StartedAt != (time.Time{}) && s.StartedAt.UnixNano() != 0 {
			hdr[i] = KeyElapsed
			i++

			hdr[i] = now.Sub(s.StartedAt)
			i++
		}
	}

	{
		hdr[i] = KeyEventType
		i++

		hdr[i] = EventType("f")
		i++
	}

	_ = s.Logger.Encoder.Encode(hdr[:i], kvs)
}

func (l *Logger) Event(kvs ...interface{}) error {
	if l == nil {
		return nil
	}

	return l.Encoder.Encode(nil, kvs)
}

func (s Span) Event(kvs ...interface{}) error {
	if s.Logger == nil {
		return nil
	}

	var hdr [2]interface{}
	i := 0

	if s.ID != (ID{}) {
		hdr[i] = KeySpan
		i++

		hdr[i] = s.ID
		i++
	}

	return s.Logger.Encoder.Encode(hdr[:i], kvs)
}

func SetLabels(ls Labels) {
	DefaultLogger.SetLabels(ls)
}

func (l *Logger) SetLabels(ls Labels) {
	if l == nil {
		return
	}

	if l.NoTime {
		l.Event(KeyLabels, ls)
		return
	}

	t := Timestamp(nano())

	l.Event(KeyTime, t, KeyLabels, ls)
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
func (l *Logger) Printf(f string, args ...interface{}) {
	newmessage(l, ID{}, 0, Format{Fmt: f, Args: args}, nil)
}

//go:noinline
func (l *Logger) Printw(msg string, kvs ...interface{}) {
	newmessage(l, ID{}, 0, Message(msg), kvs)
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
	newmessage(w.Logger, w.ID, w.d, Message(low.UnsafeBytesToString(p)), nil)

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

	var hdr [8]interface{}
	i := 0

	{
		hdr[i] = KeyEventType
		i++

		hdr[i] = EventType("m")
		i++
	}

	{
		hdr[i] = KeyMessage
		i++

		hdr[i] = name
		i++
	}

	{
		hdr[i] = "type"
		i++

		hdr[i] = typ
		i++
	}

	if help != "" {
		hdr[i] = "help"
		i++

		hdr[i] = help
		i++
	}

	_ = l.Encoder.Encode(hdr[:i], kvs)
}

func TestSetTime(t func() time.Time, ts func() int64) {
	now = t
	nano = ts
}
