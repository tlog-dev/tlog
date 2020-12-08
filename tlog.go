package tlog

import (
	"io"
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

		NewID func() ID

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

var now = time.Now

var DefaultLogger *Logger

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
		t = Timestamp(low.UnixNano())
	}

	var lc loc.PC
	if !l.NoCaller && d >= 0 {
		caller1(2+d, &lc, 1, 1)
	}

	defer l.Unlock()
	l.Lock()

	defer l.clearBuf()

	if id != (ID{}) {
		l.appendBuf("s", id)
	}

	if !l.NoTime {
		l.appendBuf("t", t)
	}
	if !l.NoCaller {
		l.appendBuf("l", lc)
	}

	if msg != nil {
		l.appendBuf("m", msg)
	}

	_ = l.Encoder.Encode(l.buf, kvs)
}

func newspan(l *Logger, par ID, n string, kvs []interface{}) (s Span) {
	if l == nil {
		return
	}

	s.Logger = l
	s.ID = l.NewID()
	s.StartedAt = now()

	var lc loc.PC
	if !l.NoCaller {
		caller1(2, &lc, 1, 1)
	}

	defer l.Unlock()
	l.Lock()

	defer l.clearBuf()

	l.appendBuf("s", s.ID)
	if par != (ID{}) {
		l.appendBuf("p", par)
	}

	if !l.NoTime {
		l.appendBuf("t", Timestamp(s.StartedAt.UnixNano()))
	}
	if !l.NoCaller {
		l.appendBuf("l", lc)
	}

	l.appendBuf("t", "s")

	if n != "" {
		l.appendBuf("m", n)
	}

	_ = l.Encoder.Encode(l.buf, kvs)

	return
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

	s.Logger.appendBuf("s", s)
	s.Logger.appendBuf("t", "f")

	if el != 0 {
		s.Logger.appendBuf("e", el)
	}

	_ = s.Logger.Encoder.Encode(s.Logger.buf, kvs)
}

func (l *Logger) Event(kvs ...[]interface{}) error {
	if l == nil {
		return nil
	}

	defer l.Unlock()
	l.Lock()

	return l.Encoder.Encode(nil, kvs...)
}

func (s Span) Event(kvs ...[]interface{}) error {
	if s.Logger == nil {
		return nil
	}

	defer s.Logger.Unlock()
	s.Logger.Lock()

	defer s.Logger.clearBuf()

	if s.ID != (ID{}) {
		s.Logger.appendBuf("s", s.ID)
	}

	return s.Logger.Encoder.Encode(s.Logger.buf, kvs...)
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

func (l *Logger) Start(n string, kvs ...interface{}) Span {
	return newspan(l, ID{}, n, kvs)
}

func (l *Logger) Spawn(par ID, n string, kvs ...interface{}) Span {
	return newspan(l, par, n, kvs)
}

func (s Span) Spawn(n string, kvs ...interface{}) Span {
	return newspan(s.Logger, s.ID, n, kvs)
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
