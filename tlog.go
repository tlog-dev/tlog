package tlog

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
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

	// Level is log level.
	// Default Level is 0. The higher level the more important the message.
	// The lower level the less important the message.
	//
	// Level is not for diciding to produce the message or not to but for fitering them while monitoring the system.
	Level int8

	// Logger is a convenient public API to produce logging events.
	// Events are packed into structs and passed to the Writer.
	// A Logger methods can be called simultaneously.
	Logger struct {
		Writer

		mu     sync.Mutex
		filter filter

		// NoCaller disables capturing caller's frame.
		NoCaller bool

		rnd    *rand.Rand
		randID func() ID
	}

	writeWrapper struct {
		Span

		// DepthCorrection is for passing Logger to another loggers. Example:
		//     log.SetOutput(l) // stdlib.
		d int
	}

	// Writer is an encoder and writer of events.
	Writer interface {
		Labels(ls Labels, sid ID) error
		SpanStarted(s SpanStart) error
		SpanFinished(f SpanFinish) error
		Message(m Message, sid ID) error
		Metric(m Metric, sid ID) error
		Meta(m Meta) error
	}

	// Span is a tracing primitive. Span usually represents some function call.
	Span struct {
		Logger *Logger

		ID ID

		StartedAt time.Time
	}

	// SpanStart is a log event.
	SpanStart struct {
		ID        ID
		Parent    ID
		StartedAt int64
		PC        PC
	}

	// SpanFinish is a log event.
	SpanFinish struct {
		ID      ID
		Elapsed int64
	}

	// Message is a log event.
	Message struct {
		PC    PC
		Time  int64
		Text  string
		Attrs Attrs
		//	Level Level
	}

	Args = []interface{}

	Attrs = []Attr

	Attr struct {
		Name  string
		Value interface{}
	}

	// Metric is a log event.
	Metric struct {
		Name   string
		Value  float64
		Labels Labels
	}

	// Meta is a log event.
	Meta struct {
		Type string

		Data Labels
	}

	// TooShortIDError is an ID parsing error.
	TooShortIDError struct {
		N int
	}
)

// for you not to import os if you don't want.
var (
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

// Log levels.
const (
	LevelInfo Level = iota
	LevelError
	LevelFatal

	// Consider using V verbosity filter.
	LevelDebug Level = -1
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

var now = time.Now

// DefaultLogger is a package interface Logger object.
var DefaultLogger = func() *Logger { l := New(NewConsoleWriter(os.Stderr, LstdFlags)); l.NoCaller = true; return l }()

var ErrorLabel = Labels{"error"}

var ( // regexp
	mtName  = "[_a-zA-Z][_a-zA-Z0-9]*"
	mtLabel = mtName + "(=[_=/a-zA-Z0-9-]*)?"

	mtNameRe  = regexp.MustCompile("^" + mtName + "$")
	mtLabelRe = regexp.MustCompile("^" + mtLabel + "$")
)

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
	if l == nil {
		return Span{}
	}

	var loc PC
	if !l.NoCaller {
		loc = Funcentry(d + 2)
	}

	s := Span{
		Logger:    l,
		StartedAt: now(),
	}

	l.mu.Lock()

	s.ID = l.randID()

	l.mu.Unlock()

	_ = l.Writer.SpanStarted(SpanStart{
		ID:        s.ID,
		Parent:    par,
		StartedAt: s.StartedAt.UnixNano(),
		PC:        loc,
	})

	return s
}

func newmessage(l *Logger, d int, lvl Level, sid ID, f string, args []interface{}, attrs Attrs) {
	if l == nil {
		return
	}

	t := now()

	var loc PC
	if !l.NoCaller {
		loc = Caller(d + 2)
	}

	var txt []byte

	if args == nil {
		if f != "" {
			txt = stringToBytes(f)
		}
	} else {
		b, wr := Getbuf()
		defer wr.Ret(&b)

		if f != "" {
			b = AppendPrintf(b, f, args...)
		} else {
			b = AppendPrintln(b, args...)
		}

		txt = b
	}

	var lattrs Attrs

	if len(attrs) != 0 {
		b, wr := GetAttrsbuf()
		defer wr.Ret(&b)

		lattrs = append(b[:0], attrs...)
	}

	_ = l.Writer.Message(
		Message{
			PC:    loc,
			Time:  t.UnixNano(),
			Text:  bytesToString(txt),
			Attrs: lattrs,
			//	Level: lvl,
		},
		sid,
	)
}

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

// AppendWriter appends writers. Multiple writers are combined into TeeWriter. Order matters.
func (l *Logger) AppendWriter(ws ...Writer) {
	if len(ws) == 0 {
		return
	}

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

// RandID returns random ID filled with Logger's random.
func (l *Logger) RandID() (id ID) {
	if l == nil {
		return
	}

	l.mu.Lock()
	id = l.randID()
	l.mu.Unlock()

	return
}

// SetLabels sets labels for default logger.
func SetLabels(ls Labels) {
	newlabels(DefaultLogger, ls, ID{})
}

// Printf builds and writes Message event.
// Arguments are handled in the manner of fmt.Printf.
func Printf(f string, args ...interface{}) {
	newmessage(DefaultLogger, 0, 0, ID{}, f, args, nil)
}

// PrintfDepth builds and writes Message event.
// d is a number of stack trace frames to skip from caller of that function. 0 is equal to Printf.
// Arguments are handled in the manner of fmt.Printf.
func PrintfDepth(d int, f string, args ...interface{}) {
	newmessage(DefaultLogger, d, 0, ID{}, f, args, nil)
}

// Panicf does the same as Printf but panics in the end.
// panic argument is fmt.Sprintf result with the func arguments.
func Panicf(f string, args ...interface{}) {
	newmessage(DefaultLogger, 0, 0, ID{}, f, args, nil)
	panic(fmt.Sprintf(f, args...))
}

// Fatalf does the same as Printf but calls os.Exit(1) in the end.
func Fatalf(f string, args ...interface{}) {
	newmessage(DefaultLogger, 0, 0, ID{}, f, args, nil)
	os.Exit(1)
}

// PrintBytes writes Message event with given text.
//
// This functions is intended to use in a really hot code.
// All possible allocs are eliminated. You should reuse buffer either.
func PrintBytes(d int, b []byte) {
	newmessage(DefaultLogger, d, 0, ID{}, bytesToString(b), nil, nil)
}

// Println does the same as Printf but formats message in fmt.Println manner.
func Println(args ...interface{}) {
	newmessage(DefaultLogger, 0, 0, ID{}, "", args, nil)
}

// Printw does the same as Printf but preserves attributes types.
// So that they can be restored and processed according to it's type.
// In contrast Printf arguments are encoded into string and can only be searched by text search.
//
// Only basic types are supported: ints, uints, string and ID.
func Printw(msg string, kv ...Attr) {
	newmessage(DefaultLogger, 0, 0, ID{}, msg, nil, kv)
}

// PrintwDepth is like Printw with Depth like in PrintfDepth.
func PrintwDepth(d int, msg string, kv ...Attr) {
	newmessage(DefaultLogger, d, 0, ID{}, msg, nil, kv)
}

func PrintRaw(d int, lvl Level, f string, args Args, attrs Attrs) {
	newmessage(DefaultLogger, d, lvl, ID{}, f, args, attrs)
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

// NewSpan creates new span by hand.
func NewSpan(l *Logger, par ID, d int) Span {
	return newspan(l, d, par)
}

// Start creates new root Span.
//
// Span must be Finished in the end.
func Start() Span {
	return newspan(DefaultLogger, 0, ID{})
}

// Spawn creates new child Span.
//
// Parent Span could be started on one machine and derived on another.
//
// Span must be Finished in the end.
func Spawn(id ID) Span {
	if id == (ID{}) {
		return Span{}
	}

	return newspan(DefaultLogger, 0, id)
}

// SpawnOrStart creates new child Span if ID is not zero and new Span overwise.
//
// Parent Span could be started on one machine and derived on another.
//
// Span must be Finished in the end.
func SpawnOrStart(id ID) Span {
	return newspan(DefaultLogger, 0, id)
}

// RegisterMetric saves metric description.
// name is fully quatified name in prometheus manner.
// typ is one of tlog.M* constants.
// help is a short description.
// labels are const labels that are attached to each Observation.
func RegisterMetric(name, typ, help string, ls Labels) {
	DefaultLogger.RegisterMetric(name, typ, help, ls)
}

// Observe records Metric event.
func Observe(name string, val float64, ls Labels) {
	_ = DefaultLogger.Metric(Metric{
		Name:   name,
		Value:  val,
		Labels: ls,
	}, ID{})
}

// SetLabels sets labels for all the following events.
func (l *Logger) SetLabels(ls Labels) {
	newlabels(l, ls, ID{})
}

// Printf writes Message event to Writer.
// Arguments are handled in the manner of fmt.Printf.
func (l *Logger) Printf(f string, args ...interface{}) {
	newmessage(l, 0, 0, ID{}, f, args, nil)
}

// PrintfDepth builds and writes Message event.
// d is a number of stack trace frames to skip from caller of that function. 0 is equal to Printf.
// Arguments are handled in the manner of fmt.Printf.
func (l *Logger) PrintfDepth(d int, f string, args ...interface{}) {
	newmessage(l, d, 0, ID{}, f, args, nil)
}

// Panicf writes Message event and panics.
// Arguments are handled in the manner of fmt.Printf.
func (l *Logger) Panicf(f string, args ...interface{}) {
	newmessage(l, 0, 0, ID{}, f, args, nil)
	panic(fmt.Sprintf(f, args...))
}

// Fatalf writes Message event and calls os.Exit(1) in the end.
// Arguments are handled in the manner of fmt.Printf.
func (l *Logger) Fatalf(f string, args ...interface{}) {
	newmessage(l, 0, 0, ID{}, f, args, nil)
	os.Exit(1)
}

// PrintBytes writes Message event with given text.
//
// This functions is intended to use in a really hot code.
// All possible allocs are eliminated. You should reuse buffer either.
func (l *Logger) PrintBytes(d int, b []byte) {
	newmessage(l, d+0, 0, ID{}, bytesToString(b), nil, nil)
}

// Println does the same as Printf but formats message in fmt.Println manner.
func (l *Logger) Println(args ...interface{}) {
	newmessage(l, 0, 0, ID{}, "", args, nil)
}

// Printw does the same as Printf but preserves attributes types.
// So that they can be restored and processed according to it's type.
// In contrast Printf arguments are encoded into string and can only be searched by text search.
//
// Only basic types are supported: ints, uints, string and ID.
func (l *Logger) Printw(msg string, kv ...Attr) {
	newmessage(l, 0, 0, ID{}, msg, nil, kv)
}

// PrintwDepth is like Printw with Depth like in PrintfDepth.
func (l *Logger) PrintwDepth(d int, msg string, kv ...Attr) {
	newmessage(l, d, 0, ID{}, msg, nil, kv)
}

// PrintRaw is a Print with all args available.
func (l *Logger) PrintRaw(d int, lvl Level, f string, args Args, attrs Attrs) {
	newmessage(l, d, lvl, ID{}, f, args, attrs)
}

// RegisterMetric saves metric description.
// name is fully quatified name in prometheus manner.
// typ is one of tlog.M* constants.
// help is a short description.
// labels are const labels that are attached to each Observation.
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

// Observe records Metric event.
func (l *Logger) Observe(n string, v float64, ls Labels) {
	_ = l.Metric(Metric{
		Name:   n,
		Value:  v,
		Labels: ls,
	}, ID{})
}

// Start creates new root Span.
//
// Span must be Finished in the end.
func (l *Logger) Start() Span {
	return newspan(l, 0, ID{})
}

// Spawn creates new child Span.
//
// Span could be started on one machine and derived on another.
//
// Span must be Finished in the end.
func (l *Logger) Spawn(id ID) Span {
	if id == (ID{}) {
		return Span{}
	}

	return newspan(l, 0, id)
}

// SpawnOrStart Spawns new child Span or create new if id is empty.
func (l *Logger) SpawnOrStart(id ID) Span {
	return newspan(l, 0, id)
}

// Migrate returns copy of the given Span that writes it's events to this Logger.
func (l *Logger) Migrate(s Span) Span {
	if s.Logger == nil {
		return Span{}
	}

	return Span{
		Logger:    l,
		ID:        s.ID,
		StartedAt: s.StartedAt,
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
// It's OK to use nil Logger, it won't crash and won't emit any events to writer.
//
// Multiple comma separated topics could be provided. Logger will be non-nil if at least one of these topics is enabled.
func (l *Logger) V(tp string) *Logger {
	if l == nil || !l.ifv(tp) {
		return nil
	}

	return l
}

// Valid checks if Logger is active.
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

// Filter returns current verbosity filter value.
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

// SetLabels sets labels associated with Span.
// Unlike Logger.SetLabels it can be set at any time and have effect on all events happened before and after.
// It can be called multiple times. Different label keys are merged.
func (s Span) SetLabels(ls Labels) {
	newlabels(s.Logger, ls, s.ID)
}

func (s Span) SetError() {
	newlabels(s.Logger, ErrorLabel, s.ID)
}

// Spawn spawns new child Span.
func (s Span) Spawn() Span {
	return newspan(s.Logger, 0, s.ID)
}

// Printf writes Message event annotated with Span ID.
// Arguments are handled in the manner of fmt.Printf.
func (s Span) Printf(f string, args ...interface{}) {
	newmessage(s.Logger, 0, 0, s.ID, f, args, nil)
}

// PrintfDepth builds and writes Message event.
// d is a number of stack trace frames to skip from caller of that function. 0 is equal to Printf.
// Arguments are handled in the manner of fmt.Printf.
func (s Span) PrintfDepth(d int, f string, args ...interface{}) {
	newmessage(s.Logger, d, 0, s.ID, f, args, nil)
}

// PrintBytes writes Message event with given text annotated with Span id.
//
// This functions is intended to use in a really hot code.
// All possible allocs are eliminated. You should reuse buffer either.
func (s Span) PrintBytes(d int, b []byte) {
	newmessage(s.Logger, d, 0, s.ID, bytesToString(b), nil, nil)
}

// Println does the same as Printf but formats message in fmt.Println manner.
func (s Span) Println(args ...interface{}) {
	newmessage(s.Logger, 0, 0, s.ID, "", args, nil)
}

// Printw does the same as Printf but preserves attributes types.
// So that they can be restored and processed according to it's type.
// In contrast Printf arguments are encoded into string and can only be searched by text search.
//
// Only basic types are supported: ints, uints, string and ID.
func (s Span) Printw(msg string, kv ...Attr) {
	newmessage(s.Logger, 0, 0, s.ID, msg, nil, kv)
}

// PrintwDepth is like Printw with Depth like in PrintfDepth.
func (s Span) PrintwDepth(d int, msg string, kv ...Attr) {
	newmessage(s.Logger, d, 0, s.ID, msg, nil, kv)
}

// PrintRaw is a Print with all different args.
func (s Span) PrintRaw(d int, lvl Level, f string, args Args, attrs Attrs) {
	newmessage(s.Logger, d, lvl, s.ID, f, args, attrs)
}

// Observe records Metric event and associates it with the Span.
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

	el := now().Sub(s.StartedAt)

	_ = s.Logger.Writer.SpanFinished(SpanFinish{
		ID:      s.ID,
		Elapsed: el.Nanoseconds(),
	})
}

// Valid checks if Span was initialized.
// Span was initialized if it was created by tlog.Start or tlog.Spawn* functions.
// Span could be empty (not initialized) if verbosity filter was false at the moment of Span creation, eg tlog.V("ignored_topic").Start().
// It's safe to call any method on not Valid Span.
func (s Span) Valid() bool { return s.Logger != nil }

func (l *Logger) IOWriter(d int) io.Writer {
	return writeWrapper{
		Span: Span{
			Logger: l,
		},
		d: d,
	}
}

// WriteWrapper returns an io.Writer interface implementation.
func (s Span) IOWriter(d int) io.Writer {
	return writeWrapper{
		Span: s,
		d:    d,
	}
}

func (w writeWrapper) Write(p []byte) (int, error) {
	if w.Logger == nil {
		return len(p), nil
	}

	newmessage(w.Logger, w.d, 0, w.ID, bytesToString(p), nil, nil)

	return len(p), nil
}

// String returns short string representation.
//
// It's not supposed to be able to recover it back to the same value as it was.
func (i ID) String() string {
	var b [16]byte
	i.FormatTo(b[:], 'v')
	return string(b[:])
}

// FullString returns full id represented as string.
func (i ID) FullString() string {
	var b [32]byte
	i.FormatTo(b[:], 'v')
	return string(b[:])
}

// IDFromBytes decodes ID from bytes slice.
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

// IDFromStringAsBytes is the same as IDFromString. It avoids alloc in IDFromString(string(b)).
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
	return fmt.Sprintf("too short id: %d, wanted %d", e.N, len(ID{}))
}

// Format is fmt.Formatter interface implementation.
// It supports width. '+' flag sets width to full ID length.
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

// FormatTo is alloc free Format alternative.
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

func (l *Logger) stdRandID() (id ID) {
	for id == (ID{}) {
		_, _ = l.rnd.Read(id[:])
	}

	return
}

func AInt(n string, v int) Attr       { return Attr{Name: n, Value: v} }
func AInt64(n string, v int64) Attr   { return Attr{Name: n, Value: v} }
func AUint64(n string, v uint64) Attr { return Attr{Name: n, Value: v} }
func AFloat(n string, v float64) Attr { return Attr{Name: n, Value: v} }
func AString(n, v string) Attr        { return Attr{Name: n, Value: v} }
func AID(n string, v ID) Attr         { return Attr{Name: n, Value: v} }
func AError(n string, err error) Attr {
	if err == nil {
		return Attr{Name: n}
	}

	return Attr{Name: n, Value: err.Error()}
}
