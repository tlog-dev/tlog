// tlog is a logger and a tracer in the same time.
//
package tlog

import (
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"time"
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
		mu    sync.Mutex
		ws    []NamedWriter
		wsbuf [4]NamedWriter

		verbosed bool

		// NoLocations disables locations capturing.
		NoLocations bool
		// DepthCorrection is for passing Logger to another loggers. Example: log.SetOutput(l) // stdlib
		DepthCorrection int
	}

	// NamedWriter is an Writer guarded by filter (Logger.V("topic")).
	NamedWriter struct {
		w      Writer
		name   string
		filter *filter
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

	concurrentRandom struct {
		mu sync.Mutex
		r  *rand.Rand
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

var ( // testable time, rand
	now    = time.Now
	randID = stdRandID
	ccrand = concurrentRandom{r: rand.New(rand.NewSource(time.Now().UnixNano()))}

	digits  = []byte("0123456789abcdef")
	digitsX = []byte("0123456789ABCDEF")
)

var ( // defaults
	DefaultLogger = New(NewConsoleWriter(os.Stderr, LstdFlags)).noLocations()
)

// New creates new Logger with given writers.
func New(ws ...interface{}) *Logger {
	l := &Logger{}
	l.ws = l.wsbuf[:0]

	l.AppendWriter(ws...)

	return l
}

func (l *Logger) AppendWriter(ws ...interface{}) {
	for i := 0; i < len(ws); i++ {
		switch w := ws[i].(type) {
		case NamedWriter:
			l.appendWriter(w)
		case Writer:
			l.appendWriter(NamedWriter{w: w})
		case string:
			if i+1 == len(ws) {
				panic(w)
			}
			nw, ok := ws[i+1].(Writer)
			if !ok {
				panic(ws[i+1])
			}
			i++
			l.appendWriter(NamedWriter{name: w, w: nw})
		default:
			panic(w)
		}
	}
}

func (l *Logger) appendWriter(w NamedWriter) {
	for i, h := range l.ws {
		if h.name != w.name {
			continue
		}

		l.ws[i].w = NewTeeWriter(h.w, w.w)

		return
	}

	l.ws = append(l.ws, w)
}

func NewNamedWriter(name, filter string, w Writer) NamedWriter {
	return NamedWriter{name: name, filter: newFilter(filter), w: w}
}

// SetLabels sets labels for default logger
func SetLabels(ls Labels) {
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
	DefaultLogger.SetNamedFilter("", f)
}

// SetNamedFilter is the same as SetFilter but changes filter with given name.
func SetNamedFilter(name, f string) {
	DefaultLogger.SetNamedFilter(name, f)
}

// Filter returns current verbosity filter for default NamedWriter in DefaultLogger.
func Filter() string {
	return DefaultLogger.NamedFilter("")
}

// NamedFilter returns current verbosity filter for given NamedWriter in DefaultLogger.
func NamedFilter(name string) string {
	return DefaultLogger.NamedFilter(name)
}

func newspan(l *Logger, par ID) Span {
	var id ID
	for id == z {
		id = randID()
	}

	var loc Location
	if !l.NoLocations {
		loc = Funcentry(l.DepthCorrection + 2)
	}

	s := Span{
		l:       l,
		ID:      id,
		Started: now(),
	}

	defer l.mu.Unlock()
	l.mu.Lock()

	for _, w := range l.ws {
		w.w.SpanStarted(s, par, loc)
	}

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
		loc = Caller(l.DepthCorrection + d + 1)
	}

	defer l.mu.Unlock()
	l.mu.Lock()

	for _, w := range l.ws {
		if !l.verbosed && w.name != "" {
			continue
		}
		w.w.Message(
			Message{
				Location: loc,
				Time:     t,
				Format:   f,
				Args:     args,
			},
			s,
		)
	}
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

// SpawnOrStart creates new child trace if id is not zero and new trace overwise.
//
// Trace could be started on one machine and derived on another.
//
// Span must be Finished in the end.
func SpawnOrStart(id ID) Span {
	if DefaultLogger == nil {
		return Span{}
	}

	return newspan(DefaultLogger, id)
}

func (l *Logger) Labels(ls Labels) {
	if l == nil {
		return
	}

	defer l.mu.Unlock()
	l.mu.Lock()

	for _, w := range l.ws {
		w.w.Labels(ls)
	}
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

// Write is an io.Writer interface implementation.
//
// It never returns any error.
func (l *Logger) Write(b []byte) (int, error) {
	newmessage(l, 1, Span{}, bytesToString(b), nil)
	return len(b), nil
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

	defer l.mu.Unlock()
	l.mu.Lock()

	any := false

	for _, w := range l.ws {
		ok := w.filter.match(tp)
		any = any || ok
	}

	if !any {
		return nil
	}

	sl := &Logger{
		verbosed:        true,
		NoLocations:     l.NoLocations,
		DepthCorrection: l.DepthCorrection,
	}
	sl.ws = sl.wsbuf[:0]

	for _, w := range l.ws {
		if w.filter.match(tp) {
			sl.ws = append(sl.ws, w)
		}
	}

	return sl
}

// SetFilter sets filter to use in V.
//
// See package.SetFilter description for details.
func (l *Logger) SetFilter(filters string) {
	l.SetNamedFilter("", filters)
}

// SetNamedFilter sets filter with given name which is used for V verbosity.
//
// See package.SetFilter description for details.
func (l *Logger) SetNamedFilter(name, filters string) {
	if l == nil {
		return
	}

	defer l.mu.Unlock()
	l.mu.Lock()

	for i, w := range l.ws {
		if w.name != name {
			continue
		}

		l.ws[i].filter = newFilter(filters)
	}
}

// Filter returns current verbosity filter value for default filter.
//
// See package.SetFilter description for details.
func (l *Logger) Filter() string {
	return l.NamedFilter("")
}

// NamedFilter returns current verbosity filter value for given filter.
//
// See package.SetFilter description for details.
func (l *Logger) NamedFilter(name string) string {
	if l == nil {
		return ""
	}

	defer l.mu.Unlock()
	l.mu.Lock()

	for _, w := range l.ws {
		if w.name != name {
			continue
		}
		if w.filter == nil {
			return ""
		}
		return w.filter.f
	}

	return ""
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
	l := s.l.v(tp)
	if l == nil {
		return Span{}
	}
	return Span{
		l:       l,
		ID:      s.ID,
		Started: s.Started,
	}
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

// Write is an io.Writer interface implementation.
//
// It never returns any error.
func (s Span) Write(b []byte) (int, error) {
	if s.ID == z {
		return len(b), nil
	}

	newmessage(s.l, 1, s, bytesToString(b), nil)

	return len(b), nil
}

// Finish writes Span finish event to Writer.
func (s Span) Finish() {
	if s.ID == z {
		return
	}

	el := now().Sub(s.Started)

	defer s.l.mu.Unlock()
	s.l.mu.Lock()

	for _, w := range s.l.ws {
		w.w.SpanFinished(s, el)
	}
}

// Valid checks if Span was initialized.
// Span was initialized if it was created by tlog.Start or tlog.Spawn* functions.
// Span could be empty (not initialized) if verbosity filter was false at the moment of Span creation, eg tlog.V("ignored_topic").Start().
func (s Span) Valid() bool { return s.l != nil && s.ID != z }

// String returns short string representation.
func (i ID) String() string {
	var b [16]byte
	i.FormatTo(b[:], 'v')
	return string(b[:])
}

// FullString returns full id in string representation.
func (i ID) FullString() string {
	var b [32]byte
	i.FormatTo(b[:], 'v')
	return string(b[:])
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

// ShouldIDFromBytes copies slice value to id without checks.
func ShouldIDFromBytes(b []byte) (id ID) {
	copy(id[:], b)
	return
}

// MustIDFromBytes copies slice value into id and panics if something is not ok.
func MustIDFromBytes(b []byte) (id ID) {
	n := copy(id[:], b)
	if n < len(id) {
		panic(id)
	}
	return
}

// IDFromString parses ID from string.
//
// If parsed string is shorter than type length result is returned as is and TooShortIDError as error value.
// You may use result if you expected short ID prefix (profuced by ID.String, for example).
func IDFromString(s string) (id ID, err error) {
	a := []byte(s)
	for i, c := range a {
		if c == '_' {
			a[i] = '0'
		}
	}
	n, err := hex.Decode(id[:], a)
	if err != nil {
		return
	}
	if n < len(id) {
		return id, TooShortIDError{N: n}
	}
	return id, nil
}

// ShouldIDFromString parses ID from string. It skips all errors.
func ShouldIDFromString(s string) (id ID) {
	a := []byte(s)
	for i, c := range a {
		if c == '_' {
			a[i] = '0'
		}
	}
	_, _ = hex.Decode(id[:], a)
	return
}

// MustIDFromString parses ID from string. It panics if something is not ok.
func MustIDFromString(s string) (id ID) {
	a := []byte(s)
	for i, c := range a {
		if c == '_' {
			a[i] = '0'
		}
	}
	n, err := hex.Decode(id[:], a)
	if err != nil {
		panic(err)
	}
	if n < len(id) {
		panic(id)
	}
	return
}

// Error is an error interface implementation.
func (e TooShortIDError) Error() string {
	return fmt.Sprintf("too short id: %d, wanted %d", e.N, len(ID{})*2)
}

// Format is fmt.Formatter interface implementation.
// It supports settings width. '+' flag sets width to full id length
func (i ID) Format(s fmt.State, c rune) {
	var buf [32]byte
	w := 16
	if W, ok := s.Width(); ok {
		w = W
	}
	if s.Flag('+') {
		w = 2 * len(i)
	}
	i.FormatTo(buf[:w], c)
	_, _ = s.Write(buf[:w])
}

func (i ID) FormatTo(b []byte, f rune) {
	if (f == 'v' || f == 'V') && i == z {
		for j := 0; j < 2*len(i) && j < len(b); j++ {
			b[j] = '_'
		}
		return
	}
	dg := digits
	if f == 'X' {
		dg = digitsX
	}
	for j := 0; j < 2*len(i) && j < len(b); j += 2 {
		q := i[j/2]
		b[j] = dg[q>>4&0xf]
		if j+1 < len(b) {
			b[j+1] = dg[q&0xf]
		}
	}
}

func (i ID) FormatQuotedTo(b []byte, f rune) {
	if len(b) < 3 {
		panic(len(b))
	}
	b[0] = '"'
	b[len(b)-1] = '"'
	i.FormatTo(b[1:len(b)-1], f)
}

// AbsTime converts Message Time field from nanoseconds from Unix epoch to time.Time
func (m *Message) AbsTime() time.Time {
	return time.Unix(0, int64(m.Time))
}

func stdRandID() (id ID) {
	ccrand.mu.Lock()
	_, _ = ccrand.r.Read(id[:]) //nolint:gosec
	ccrand.mu.Unlock()
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
