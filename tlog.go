// tlog is a logger and a tracer in the same time.
//
package tlog

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"regexp"
	"sync"
	"time"
	"unsafe"
)

type (
	// ID is a Span ID.
	ID [16]byte

	// Printfer is an interface to print to *Logger and to Span in the same time.
	Printfer interface {
		Printf(string, ...interface{})
	}

	// Logger is an logging handler that creates logging events and passes them to the Writer.
	// A Logger methods can be called simultaneously.
	Logger struct {
		Writer

		mu     sync.Mutex
		filter filter

		// NoLocations disables locations capturing.
		NoLocations bool

		// DepthCorrection is for passing Logger to another loggers. Example:
		//     log.SetOutput(l) // stdlib.
		// Have effect on Write function only.
		DepthCorrection int

		rnd    *rand.Rand
		randID func() ID
	}

	// Writer is an general encoder and writer of events.
	Writer interface {
		Labels(ls Labels, sid ID) error
		SpanStarted(s SpanStart) error
		SpanFinished(f SpanFinish) error
		Message(m Message, sid ID) error
		Metric(m Metric, sid ID) error
		Meta(m Meta) error
	}

	SpanStart struct {
		ID       ID
		Parent   ID
		Started  int64
		Location Location
	}

	SpanFinish struct {
		ID      ID
		Elapsed int64
	}

	// Message is an Log event.
	Message struct {
		Location Location
		Time     int64
		Text     []byte
	}

	Metric struct {
		Name   string
		Value  float64
		Labels Labels
	}

	// Span is an tracing primitive. Span usually represents some function call.
	Span struct {
		Logger *Logger

		ID ID

		Started int64
	}

	Meta struct {
		Type string

		Data Labels
	}

	TooShortIDError struct {
		N int
	}
)

var ( // for you not to import os if you don't want
	Stderr = os.Stderr
	Stdout = os.Stdout
)

// ConsoleWriter flags. Similar to log.Logger flags.
const ( // console writer flags
	Ldate = 1 << iota
	Ltime
	Lseconds
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

const ( // metric types
	MCounter   = "counter"
	MGauge     = "gauge"
	MSummary   = "summary"
	MUntyped   = "untyped"
	MHistogram = "histogram"
	Mempty     = ""
)

var now = func() int64 { return time.Now().UnixNano() }

var DefaultLogger = func() *Logger { l := New(NewConsoleWriter(os.Stderr, LstdFlags)); l.NoLocations = true; return l }()

var ErrorLabel = Labels{"error"}

var ( // regexp
	mtName  = "[_a-zA-Z][_a-zA-Z0-9]*"
	mtLabel = mtName + "(=[_=/a-zA-Z0-9]*)?"

	mtNameRe  = regexp.MustCompile(mtName)
	mtLabelRe = regexp.MustCompile(mtLabel)
)

// New creates new Logger with given writers.
func New(ws ...Writer) *Logger {
	l := &Logger{
		rnd: rand.New(rand.NewSource(time.Now().UnixNano())), //nolint:gosec
	}
	l.randID = l.stdRandID

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

func (l *Logger) AppendWriter(ws ...Writer) {
	switch w := l.Writer.(type) {
	case DiscardWriter:
		if len(ws) == 1 {
			l.Writer = ws[0]
		} else {
			l.Writer = NewTeeWriter(ws...)
		}

	case TeeWriter:
		l.Writer = append(w, ws...)

	default:
		tw := NewTeeWriter(w)
		l.Writer = append(tw, ws...)
	}
}

// SetLabels sets labels for default logger.
func SetLabels(ls Labels) {
	newlabels(DefaultLogger, ls, ID{})
}

// Printf writes logging Message.
// Arguments are handled in the manner of fmt.Printf.
func Printf(f string, args ...interface{}) {
	newmessage(DefaultLogger, 0, ID{}, f, args)
}

// PrintfDepth writes logging Message.
// Depth is a number of stack trace frames to skip from caller of that function. 0 is equal to Printf.
// Arguments are handled in the manner of fmt.Printf.
func PrintfDepth(d int, f string, args ...interface{}) {
	newmessage(DefaultLogger, d, ID{}, f, args)
}

// Panicf does the same as Printf but panics in the end.
// panic argument is fmt.Sprintf result with func arguments.
func Panicf(f string, args ...interface{}) {
	newmessage(DefaultLogger, 0, ID{}, f, args)
	panic(fmt.Sprintf(f, args...))
}

// Fatalf does the same as Printf but calls os.Exit(1) in the end.
func Fatalf(f string, args ...interface{}) {
	newmessage(DefaultLogger, 0, ID{}, f, args)
	os.Exit(1)
}

// PrintRaw writes logging Message with given text.
//
// This functions is intended to use in a really hot code.
// All possible allocs are eliminated. You should reuse buffer either.
func PrintRaw(d int, b []byte) {
	newmessage(DefaultLogger, d, ID{}, bytesToString(b), nil)
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
	if !DefaultLogger.ifv(tp) {
		return nil
	}

	return DefaultLogger
}

func If(tp string) bool {
	return DefaultLogger.ifv(tp)
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

// Filter returns current verbosity filter for default NamedWriter in DefaultLogger.
func Filter() string {
	return DefaultLogger.Filter()
}

func newlabels(l *Logger, ls Labels, sid ID) {
	if l == nil {
		return
	}

	for _, l := range ls {
		if !mtLabelRe.MatchString(l) {
			panic("bad label: " + l + ", expected: " + mtLabel)
		}
	}

	_ = l.Writer.Labels(ls, sid)
}

func newspan(l *Logger, d int, par ID) Span {
	var loc Location
	if !l.NoLocations {
		loc = Funcentry(d + 2)
	}

	s := Span{
		Logger:  l,
		Started: now(),
	}

	l.mu.Lock()

	s.ID = l.randID()

	l.mu.Unlock()

	_ = l.Writer.SpanStarted(SpanStart{
		ID:       s.ID,
		Parent:   par,
		Started:  s.Started,
		Location: loc,
	})

	return s
}

func newmessage(l *Logger, d int, sid ID, f string, args []interface{}) {
	if l == nil {
		return
	}

	t := now()

	var loc Location
	if !l.NoLocations {
		loc = Caller(d + 2)
	}

	var txt []byte

	if args == nil {
		if f != "" {
			txt = stringToBytes(f)
		}
	} else {
		b, wr := getbuf()
		defer retbuf(b, wr)

		if f == "" {
			b = AppendPrintln(b, args...)
		} else {
			b = AppendPrintf(b, f, args...)
		}

		txt = b
	}

	_ = l.Writer.Message(
		Message{
			Location: loc,
			Time:     t,
			Text:     txt,
		},
		sid,
	)
}

func NewSpan(l *Logger, par ID, d int) Span {
	if l == nil {
		return Span{}
	}

	return newspan(l, d, par)
}

// Start creates new root trace.
//
// Span must be Finished in the end.
func Start() Span {
	if DefaultLogger == nil {
		return Span{}
	}

	return newspan(DefaultLogger, 0, ID{})
}

// Spawn creates new child trace.
//
// Trace could be started on one machine and derived on another.
//
// Span must be Finished in the end.
func Spawn(id ID) Span {
	if DefaultLogger == nil || id == (ID{}) {
		return Span{}
	}

	return newspan(DefaultLogger, 0, id)
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

	return newspan(DefaultLogger, 0, id)
}

func RegisterMetric(name, typ, help string, ls Labels) {
	DefaultLogger.RegisterMetric(name, typ, help, ls)
}

func Observe(n string, v float64, ls Labels) {
	_ = DefaultLogger.Metric(Metric{
		Name:   n,
		Labels: ls,
		Value:  v,
	}, ID{})
}

func (l *Logger) SetLabels(ls Labels) {
	newlabels(l, ls, ID{})
}

// Printf writes logging Message to Writer.
// Arguments are handled in the manner of fmt.Printf.
func (l *Logger) Printf(f string, args ...interface{}) {
	newmessage(l, 0, ID{}, f, args)
}

// PrintfDepth writes logging Message.
// Depth is a number of stack trace frames to skip from caller of that function. 0 is equal to Printf.
// Arguments are handled in the manner of fmt.Printf.
func (l *Logger) PrintfDepth(d int, f string, args ...interface{}) {
	newmessage(l, d, ID{}, f, args)
}

// Panicf writes logging Message and panics.
// Arguments are handled in the manner of fmt.Printf.
func (l *Logger) Panicf(f string, args ...interface{}) {
	newmessage(l, 0, ID{}, f, args)
	panic(fmt.Sprintf(f, args...))
}

// Panicf writes logging Message and calls os.Exit(1) in the end.
// Arguments are handled in the manner of fmt.Printf.
func (l *Logger) Fatalf(f string, args ...interface{}) {
	newmessage(l, 0, ID{}, f, args)
	os.Exit(1)
}

// PrintRaw writes logging Message with given text.
//
// This functions is intended to use in a really hot code.
// All possible allocs are eliminated. You should reuse buffer either.
func (l *Logger) PrintRaw(d int, b []byte) {
	newmessage(l, d+0, ID{}, bytesToString(b), nil)
}

func (l *Logger) Println(args ...interface{}) {
	newmessage(l, 0, ID{}, "", args)
}

// Write is an io.Writer interface implementation.
//
// It never returns any error.
func (l *Logger) Write(b []byte) (int, error) {
	if l == nil {
		return len(b), nil
	}

	newmessage(l, l.DepthCorrection, ID{}, bytesToString(b), nil)

	return len(b), nil
}

func (l *Logger) RegisterMetric(name, typ, help string, ls Labels) {
	if !mtNameRe.MatchString(name) {
		panic("bad metric name: " + name + ", expected: " + mtName)
	}
	for _, l := range ls {
		if !mtLabelRe.MatchString(l) {
			panic("bad label: " + l + ", expected: " + mtLabel)
		}
	}

	_ = l.Meta(Meta{
		Type: "metric_desc",
		Data: append(
			Labels{
				"name=" + name,
				"type=" + typ,
				"help=" + help,
				"labels",
			},
			ls...),
	})
}

func (l *Logger) Observe(n string, v float64, ls Labels) {
	_ = l.Metric(Metric{
		Name:   n,
		Value:  v,
		Labels: ls,
	}, ID{})
}

// Start creates new root trace.
//
// Span must be Finished in the end.
func (l *Logger) Start() Span {
	if l == nil {
		return Span{}
	}

	return newspan(l, 0, ID{})
}

// Spawn creates new child trace.
//
// Trace could be started on one machine and derived on another.
//
// Span must be Finished in the end.
func (l *Logger) Spawn(id ID) Span {
	if l == nil || id == (ID{}) {
		return Span{}
	}

	return newspan(l, 0, id)
}

func (l *Logger) SpawnOrStart(id ID) Span {
	if l == nil {
		return Span{}
	}

	return newspan(l, 0, id)
}

func (l *Logger) Migrate(s Span) Span {
	return Span{
		Logger:  l,
		ID:      s.ID,
		Started: s.Started,
	}
}

// If checks if some of topics enabled.
func (l *Logger) If(tp string) bool {
	return l.ifv(tp)
}

func (l *Logger) ifv(tp string) (ok bool) {
	if l == nil {
		return false
	}

	l.mu.Lock()

	ok = l.filter.match(tp)

	l.mu.Unlock()

	return ok
}

// V checks if one of topics in tp is enabled and returns default Logger or nil.
//
// It's OK to use nil Logger, it won't crash and won't emit eny Messages to writer.
//
// Multiple comma separated topics could be passed. Logger will be non-nil if at least one of these topics is enabled.
func (l *Logger) V(tp string) *Logger {
	if l == nil || !l.ifv(tp) {
		return nil
	}

	return l
}

// Valid checks if Logger is not nil and was not disabled by filter.
// It's safe to call any method from not Valid Logger.
func (l *Logger) Valid() bool { return l != nil }

// SetFilter sets filter to use in V.
//
// See package.SetFilter description for details.
func (l *Logger) SetFilter(filters string) {
	if l == nil {
		return
	}

	l.mu.Lock()

	l.filter = newFilter(filters)

	l.mu.Unlock()
}

// Filter returns current verbosity filter value for default filter.
//
// See package.SetFilter description for details.
func (l *Logger) Filter() string {
	if l == nil {
		return ""
	}

	defer l.mu.Unlock()
	l.mu.Lock()

	return l.filter.f
}

// V checks if one of topics in tp is enabled and returns the same Span or empty.
//
// It return empty Span if you call V on empty Span.
//
// Multiple comma separated topics could be passed. Logger will be non-nil if at least one of these topics is enabled.
func (s Span) V(tp string) Span {
	if !s.Logger.ifv(tp) {
		return Span{}
	}

	return s
}

func (s Span) If(tp string) bool {
	return s.Logger.ifv(tp)
}

func (s Span) SetLabels(ls Labels) {
	newlabels(s.Logger, ls, s.ID)
}

func (s Span) SetError() {
	newlabels(s.Logger, ErrorLabel, s.ID)
}

// Spawn spawns new child Span.
func (s Span) Spawn() Span {
	if s.Logger == nil {
		return Span{}
	}

	return newspan(s.Logger, 0, s.ID)
}

// Printf writes logging Message annotated with trace id.
// Arguments are handled in the manner of fmt.Printf.
func (s Span) Printf(f string, args ...interface{}) {
	newmessage(s.Logger, 0, s.ID, f, args)
}

// PrintfDepth writes logging Message.
// Depth is a number of stack trace frames to skip from caller of that function. 0 is equal to Printf.
// Arguments are handled in the manner of fmt.Printf.
func (s Span) PrintfDepth(d int, f string, args ...interface{}) {
	newmessage(s.Logger, d, s.ID, f, args)
}

// PrintRaw writes logging Message with given text annotated with trace id.
//
// This functions is intended to use in a really hot code.
// All possible allocs are eliminated. You should reuse buffer either.
func (s Span) PrintRaw(d int, b []byte) {
	newmessage(s.Logger, d, s.ID, bytesToString(b), nil)
}

func (s Span) Println(args ...interface{}) {
	newmessage(s.Logger, 0, s.ID, "", args)
}

// Write is an io.Writer interface implementation.
//
// It never returns any error.
func (s Span) Write(b []byte) (int, error) {
	if s.Logger == nil {
		return len(b), nil
	}

	newmessage(s.Logger, s.Logger.DepthCorrection, s.ID, bytesToString(b), nil)

	return len(b), nil
}

func (s Span) Observe(n string, v float64, ls Labels) {
	_ = s.Logger.Metric(Metric{
		Name:   n,
		Labels: ls,
		Value:  v,
	}, s.ID)
}

// Finish writes Span finish event to Writer.
func (s Span) Finish() {
	if s.Logger == nil {
		return
	}

	el := now() - s.Started

	_ = s.Logger.Writer.SpanFinished(SpanFinish{
		ID:      s.ID,
		Elapsed: el,
	})
}

// Valid checks if Span was initialized.
// Span was initialized if it was created by tlog.Start or tlog.Spawn* functions.
// Span could be empty (not initialized) if verbosity filter was false at the moment of Span creation, eg tlog.V("ignored_topic").Start().
// It's safe to call any method on not Valid Span.
func (s Span) Valid() bool { return s.Logger != nil }

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
		err = TooShortIDError{N: n}
	}

	return
}

// IDFromString parses ID from string.
//
// If parsed string is shorter than type length result is returned as is and TooShortIDError as error value.
// You may use result if you expected short ID prefix (profuced by ID.String, for example).
func IDFromString(s string) (id ID, err error) {
	if "________________________________"[:len(s)] == s {
		return
	}

	var i int
	var c byte
	for ; i < len(s); i++ {
		switch {
		case s[i] == '_':
			continue
		case '0' <= s[i] && s[i] <= '9':
			c = s[i] - '0'
		case 'a' <= s[i] && s[i] <= 'f':
			c = s[i] - 'a' + 10
		default:
			err = hex.InvalidByteError(s[i])
			return
		}

		if i&1 == 0 {
			c <<= 4
		}

		id[i>>1] |= c
	}

	if i < 2*len(id) {
		err = TooShortIDError{N: i / 2}
	}

	return
}

func IDFromStringAsBytes(s []byte) (id ID, err error) {
	if bytes.Equal([]byte("________________________________")[:len(s)], s) {
		return
	}

	n, err := hex.Decode(id[:], s)
	if err != nil {
		return
	}

	if n < len(id) {
		return id, TooShortIDError{N: n}
	}

	return id, nil
}

// ShouldID wraps IDFrom* call and skips error if any.
func ShouldID(id ID, err error) ID {
	return id
}

// MustID wraps IDFrom* call and panics if error occurred.
func MustID(id ID, err error) ID {
	if err != nil {
		panic(err)
	}

	return id
}

// Error is an error interface implementation.
func (e TooShortIDError) Error() string {
	return fmt.Sprintf("too short id: %d, wanted %d", e.N, len(ID{})*2)
}

// Format is fmt.Formatter interface implementation.
// It supports width. '+' flag sets width to full id length.
func (i ID) Format(s fmt.State, c rune) {
	var buf0 [32]byte
	buf1 := buf0[:]
	buf := *(*[]byte)(noescape(unsafe.Pointer(&buf1)))

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
	if i == (ID{}) {
		if f == 'v' || f == 'V' {
			copy(b, "________________________________")
		} else {
			copy(b, "00000000000000000000000000000000")
		}
		return
	}

	const digitsx = "0123456789abcdef"
	const digitsX = "0123456789ABCDEF"

	dg := digitsx
	if f == 'X' || f == 'V' {
		dg = digitsX
	}

	m := len(b)
	if 2*len(i) < m {
		m = 2 * len(i)
	}

	ji := 0
	for j := 0; j+1 < m; j += 2 {
		b[j] = dg[i[ji]>>4]
		b[j+1] = dg[i[ji]&0xf]
		ji++
	}

	if m&1 == 1 {
		b[m-1] = dg[i[m>>1]>>4]
	}
}

func (i ID) MarshalJSON() (d []byte, err error) {
	var b [34]byte

	b[0] = '"'
	b[len(b)-1] = '"'

	i.FormatTo(b[1:len(b)-1], 'x')

	return b[:], nil
}

func (i *ID) UnmarshalJSON(b []byte) error {
	if b[0] != '"' || b[len(b)-1] != '"' {
		return errors.New("bad format")
	}

	q, err := IDFromString(string(b[1 : len(b)-1]))
	if err != nil {
		return err
	}

	*i = q

	return nil
}

func (l *Logger) stdRandID() (id ID) {
	if l == nil {
		return
	}

	for id == (ID{}) {
		_, _ = l.rnd.Read(id[:])
	}

	return
}
