// tlog is a logger and a tracer in the same time.
//
package tlog

import (
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

type (
	// ID is a Span ID
	ID [16]byte

	// Printfer is an interface to print to *Logger and to Span in the same time.
	Printfer interface {
		Printf(string, ...interface{})
	}

	// Logger is an logging handler that creates logging events and passes them to the Writer.
	// A Logger can be called simultaneously if Writer supports it. Writers from this package does.
	Logger struct {
		Writer
		filter *filter
		// NoLocations disables locations capturing.
		NoLocations bool
	}

	// Writer is an general encoder and writer of events.
	Writer interface {
		Labels(ls Labels)
		SpanStarted(s Span, parent ID, l Location)
		SpanFinished(s Span, el time.Duration)
		Message(l Message, s Span)
	}

	// Message is an Log event.
	Message struct {
		Location Location
		Time     time.Duration
		Format   string
		Args     []interface{}
	}

	// Span is an tracing primitive. Span represents some function call.
	Span struct {
		l *Logger

		ID ID

		Started time.Time
	}

	TooShortIDError struct {
		N int
	}
)

var ( // ZeroID
	ZeroID ID // to compare with
	z      ID
)

// ConsoleWriter flags. Similar to log.Logger flags.
const ( // console writer flags
	Ldate = 1 << iota
	Ltime
	Lmilliseconds
	Lmicroseconds
	Lshortfile
	Llongfile
	Ltypefunc // pkg.(*Type).Func
	Lfuncname // Func
	LUTC
	Lspans       // print Span start and finish events
	Lmessagespan // add Span ID to trace messages
	LstdFlags    = Ldate | Ltime
	LdetFlags    = Ldate | Ltime | Lmicroseconds | Lshortfile
	Lnone        = 0
)

// Shortcuts for Logger filters and V topics.
const ( // log levels
	CriticalLevel = "critical"
	ErrorLevel    = "error"
	InfoLevel     = "info"
	DebugLevel    = "debug"
	TraceLevel    = "trace"
)

var ( // testable time, rand
	now    = time.Now
	randID = stdRandID

	digits  = []byte("0123456789abcdef")
	digitsX = []byte("0123456789ABCDEF")
)

var ( // defaults
	DefaultLogger = New(NewConsoleWriter(os.Stderr, LstdFlags)).noLocations()
)

// New creates new Logger with given writers.
func New(ws ...Writer) *Logger {
	l := &Logger{}

	switch len(ws) {
	case 0:
		l.Writer = Discard{}
	case 1:
		l.Writer = ws[0]
	default:
		l.Writer = NewTeeWriter(ws...)
	}

	return l
}

func (l *Logger) AppendWriter(ws ...Writer) {
	switch w := l.Writer.(type) {
	case *TeeWriter:
		w.Writers = append(w.Writers, ws...)
	default:
		l.Writer = &TeeWriter{Writers: append([]Writer{l.Writer}, ws...)}
	}
}

// SetLabels sets labels for default logger
func SetLabels(ls Labels) {
	if DefaultLogger == nil {
		return
	}
	DefaultLogger.Labels(ls)
}

// Printf writes logging Message.
// Arguments are handled in the manner of fmt.Printf.
func Printf(f string, args ...interface{}) {
	newmessage(DefaultLogger, 1, Span{}, f, args)
}

// PrintfDepth writes logging Message.
// Depth is a number of stack trace frames to skip from caller of that function. 0 is equal to Printf.
// Arguments are handled in the manner of fmt.Printf.
func PrintfDepth(d int, f string, args ...interface{}) {
	newmessage(DefaultLogger, d+1, Span{}, f, args)
}

// Panicf does the same as Printf but panics in the end.
// panic argument is fmt.Sprintf result with func arguments.
func Panicf(f string, args ...interface{}) {
	newmessage(DefaultLogger, 1, Span{}, f, args)
	panic(fmt.Sprintf(f, args...))
}

// Fatalf does the same as Printf but calls os.Exit(1) in the end.
func Fatalf(f string, args ...interface{}) {
	newmessage(DefaultLogger, 1, Span{}, f, args)
	os.Exit(1)
}

// PrintRaw writes logging Message with given text.
//
// This functions is intended to use in a really hot code.
// All possible allocs are eliminated. You should reuse buffer either.
func PrintRaw(d int, b []byte) {
	newmessage(DefaultLogger, d+1, Span{}, bytesToString(b), nil)
}

// V checks if topic tp is enabled and returns default Logger or nil.
//
// It's OK to use nil Logger, it wonn't crash and won't emit eny Messages to writer.
//
// Multiple comma separated topics could be passed. Logger will be non-nil if at least one of these topics is enabled.
//
// Usecases:
//     tlog.V("write").Printf("%d bytes written to address %v", n, addr)
//
//     if l := tlog.V("detailed"); l != nil {
//         c := 1 + 2 // do complex computations here
//         l.Printf("use result: %d")
//     }
func V(tp string) *Logger {
	return DefaultLogger.v(tp)
}

// SetFilter sets filter to use in V.
//
// Filter is a comma separated chain of rules.
// Each rule is applyed to result of previous rule and adds or removes some locations.
// Rule started with '!' excludes matching locations.
//
// Each rule is one of: topic (some word you used in V argument)
//     error
//     networking
//     send
//     encryption
//
// and location (directory, file, function)
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
//     p2p/conn.go=read+write - multiple topics in the location are separated by '+'
//
// Example
//     module,!module/file.go,funcInFile
//
// SetFilter can be called simultaneously with V.
func SetFilter(f string) {
	DefaultLogger.SetFilter(f)
}

// Filter returns current verbosity filter for DefaultLogger.
func Filter() string {
	return DefaultLogger.Filter()
}

// SetLogLevel is a shortcut for SetFilter with one of *Filter constants
func SetLogLevel(l int) {
	DefaultLogger.SetLogLevel(l)
}

func newspan(l *Logger, par ID) Span {
	var id ID
	for id == z {
		id = randID()
	}

	var loc Location
	if !l.NoLocations {
		loc = Funcentry(2)
	}

	s := Span{
		l:       l,
		ID:      id,
		Started: now(),
	}

	l.SpanStarted(s, par, loc)

	return s
}

func newmessage(l *Logger, d int, s Span, f string, args []interface{}) {
	if l == nil {
		return
	}

	var t time.Duration
	if s.ID == z {
		t = time.Duration(now().UnixNano())
	} else {
		t = now().Sub(s.Started)
	}

	var loc Location
	if !l.NoLocations {
		loc = Caller(d + 1)
	}

	l.Message(
		Message{
			Location: loc,
			Time:     t,
			Format:   f,
			Args:     args,
		},
		s,
	)
}

// Start creates new root trace.
//
// Span must be Finished in the end.
func Start() Span {
	if DefaultLogger == nil {
		return Span{}
	}

	return newspan(DefaultLogger, z)
}

// Spawn creates new child trace.
//
// Trace could be started on one machine and derived on another.
//
// Span must be Finished in the end.
func Spawn(id ID) Span {
	if DefaultLogger == nil || id == z {
		return Span{}
	}

	return newspan(DefaultLogger, id)
}

// Printf writes logging Message to Writer.
// Arguments are handled in the manner of fmt.Printf.
func (l *Logger) Printf(f string, args ...interface{}) {
	newmessage(l, 1, Span{}, f, args)
}

// PrintfDepth writes logging Message.
// Depth is a number of stack trace frames to skip from caller of that function. 0 is equal to Printf.
// Arguments are handled in the manner of fmt.Printf.
func (l *Logger) PrintfDepth(d int, f string, args ...interface{}) {
	newmessage(l, d+1, Span{}, f, args)
}

// Panicf writes logging Message and panics.
// Arguments are handled in the manner of fmt.Printf.
func (l *Logger) Panicf(f string, args ...interface{}) {
	newmessage(l, 1, Span{}, f, args)
	panic(fmt.Sprintf(f, args...))
}

// Panicf writes logging Message and calls os.Exit(1) in the end.
// Arguments are handled in the manner of fmt.Printf.
func (l *Logger) Fatalf(f string, args ...interface{}) {
	newmessage(l, 1, Span{}, f, args)
	os.Exit(1)
}

// PrintRaw writes logging Message with given text.
//
// This functions is intended to use in a really hot code.
// All possible allocs are eliminated. You should reuse buffer either.
func (l *Logger) PrintRaw(d int, b []byte) {
	newmessage(l, d+1, Span{}, bytesToString(b), nil)
}

// Start creates new root trace.
//
// Span must be Finished in the end.
func (l *Logger) Start() Span {
	if l == nil {
		return Span{}
	}

	return newspan(l, z)
}

// Spawn creates new child trace.
//
// Trace could be started on one machine and derived on another.
//
// Span must be Finished in the end.
func (l *Logger) Spawn(id ID) Span {
	if l == nil || id == z {
		return Span{}
	}

	return newspan(l, id)
}

// V checks if one of topics in tp is enabled and returns default Logger or nil.
//
// It's OK to use nil Logger, it won't crash and won't emit eny Messages to writer.
//
// Multiple comma separated topics could be passed. Logger will be non-nil if at least one of these topics is enabled.
func (l *Logger) V(tp string) *Logger {
	return l.v(tp)
}

func (l *Logger) v(tp string) *Logger {
	if l == nil {
		return nil
	}
	f := (*filter)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&l.filter))))
	if !f.match(tp) {
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
	var f *filter
	if filters != "" {
		f = newFilter(filters)
	}
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&l.filter)), unsafe.Pointer(f))
}

// Filter returns current verbosity filter value
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

// SetLogLevel is a shortcut for SetFilter with one of *Filter constants
func (l *Logger) SetLogLevel(lev int) {
	switch {
	case lev <= 0:
		l.SetFilter("")
	case lev == 1:
		l.SetFilter(CriticalLevel)
	case lev == 2:
		l.SetFilter(ErrorLevel)
	case lev == 3:
		l.SetFilter(InfoLevel)
	case lev == 4:
		l.SetFilter(DebugLevel)
	default:
		l.SetFilter(TraceLevel)
	}
}

func (l *Logger) noLocations() *Logger {
	l.NoLocations = true
	return l
}

// V checks if one of topics in tp is enabled and returns the same Span or empty.
//
// It return empty Span if you call V on empty Span.
//
// Multiple comma separated topics could be passed. Logger will be non-nil if at least one of these topics is enabled.
func (s Span) V(tp string) Span {
	if s.l.v(tp) == nil {
		return Span{}
	}
	return s
}

// Printf writes logging Message annotated with trace id.
// Arguments are handled in the manner of fmt.Printf.
func (s Span) Printf(f string, args ...interface{}) {
	if s.ID == z {
		return
	}

	newmessage(s.l, 1, s, f, args)
}

// PrintfDepth writes logging Message.
// Depth is a number of stack trace frames to skip from caller of that function. 0 is equal to Printf.
// Arguments are handled in the manner of fmt.Printf.
func (s Span) PrintfDepth(d int, f string, args ...interface{}) {
	newmessage(s.l, d+1, s, f, args)
}

// PrintRaw writes logging Message with given text annotated with trace id.
//
// This functions is intended to use in a really hot code.
// All possible allocs are eliminated. You should reuse buffer either.
func (s Span) PrintRaw(d int, b []byte) {
	if s.ID == z {
		return
	}

	newmessage(s.l, d+1, s, bytesToString(b), nil)
}

// Finish writes Span finish event to Writer.
func (s Span) Finish() {
	if s.ID == z {
		return
	}

	el := now().Sub(s.Started)
	s.l.SpanFinished(s, el)
}

// Valid checks if Span was initialized.
// Span was initialized if it was created by tlog.Start or tlog.Spawn* functions.
// Span could be empty (not initialized) if verbosity filter was false at the moment of Span creation, eg tlog.V("ignored_topic").Start().
func (s Span) Valid() bool { return s.l != nil && s.ID != z }

// String returns short string representation.
func (i ID) String() string {
	if i == z {
		return "________________"
	}
	return fmt.Sprintf("%x", i)
}

// FullString returns full id in string representation.
func (i ID) FullString() string {
	if i == z {
		return "________________________________"
	}
	return fmt.Sprintf("%+x", i)
}

// IDFromBytes checks slice length and casts to ID type (copying).
//
// If byte slice is shorter than type length result is returned as is and TooShortIDError as error value.
// You may use result if you expected short ID prefix.
func IDFromBytes(b []byte) (id ID, err error) {
	n := copy(id[:], b)
	if n < len(id) {
		return id, TooShortIDError{N: n}
	}
	return id, nil
}

// IDFromString parses ID from string.
//
// If parsed string is shorter than type length result is returned as is and TooShortIDError as error value.
// You may use result if you expected short ID prefix (profuced by ID.String, for example).
func IDFromString(s string) (id ID, err error) {
	n, err := hex.Decode(id[:], []byte(s))
	if err != nil {
		return
	}
	if n < len(id) {
		return id, TooShortIDError{N: n}
	}
	return id, nil
}

// Error is en error interface implementation.
func (e TooShortIDError) Error() string {
	return fmt.Sprintf("too short id: %d, wanted %d", e.N, len(ID{}))
}

// Format is fmt.Formatter interface implementation.
// It supports settings width. '+' flag sets width to full id length
func (i ID) Format(s fmt.State, c rune) {
	var buf [32]byte
	l := 8
	if w, ok := s.Width(); ok {
		l = w / 2
	}
	if s.Flag('+') {
		l = len(i)
	}
	if l == 0 {
		l = 1
	}
	i.FormatTo(buf[:], c)
	_, _ = s.Write(buf[:2*l])
}

func (i ID) FormatTo(b []byte, f rune) {
	if len(b) < 2*len(z) {
		panic(len(b))
	}
	dg := digits
	if f == 'X' {
		dg = digitsX
	}
	for j := 0; j < 2*len(z); j += 2 {
		q := i[j/2]
		b[j] = dg[q>>4&0xf]
		b[j+1] = dg[q&0xf]
	}
}

func (i ID) FormatQuotedTo(b []byte, f rune) {
	if len(b) < 2+2*len(z) {
		panic(len(b))
	}
	b[0] = '"'
	b[33] = '"'
	i.FormatTo(b[1:], f)
}

// AbsTime converts Message Time field from nanoseconds from Unix epoch to time.Time
func (m *Message) AbsTime() time.Time {
	return time.Unix(0, int64(m.Time))
}

func stdRandID() (id ID) {
	_, err := rand.Read(id[:]) //nolint:gosec
	if err != nil {
		panic(err)
	}
	return
}

func testRandID() func() ID {
	var mu sync.Mutex
	rnd := rand.New(rand.NewSource(0))

	return func() (id ID) {
		defer mu.Unlock()
		mu.Lock()

		_, err := rnd.Read(id[:])
		if err != nil {
			panic(err)
		}
		return
	}
}
