// tlog is a logger and a tracer in one package.
//
package tlog

import (
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

type (
	// ID is an Span ID
	ID int64

	// Labels is a set of labels with optional values.
	//
	// By design Labels contains state diff not state itself.
	// So if you want to delete some label you should use Del method to add special thumbstone value.
	Labels []string

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

		Flags int
	}

	// Rand is an interface for rand.Rand. It's intended mostly for testing purpose.
	// It's expected to support simultaneous calls.
	Rand interface {
		Int63() int64
	}

	concurrentRand struct {
		mu  sync.Mutex
		rnd Rand
	}
)

// Span flags.
const ( // span flags
	FlagError = 1 << iota

	FlagNone = 0
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

var ( // time, rand
	now      = time.Now
	rnd Rand = &concurrentRand{rnd: rand.New(rand.NewSource(now().UnixNano()))}

	digits = []byte("0123456789abcdef")
)

var ( // defaults
	DefaultLogger = New(NewConsoleWriter(os.Stderr, LstdFlags))
)

// FillLabelsWithDefaults creates Labels and fills _hostname and _pid labels with current values.
func FillLabelsWithDefaults(labels ...string) Labels {
	ll := make(Labels, 0, len(labels))

	for _, lab := range labels {
		switch {
		case strings.HasPrefix(lab, "_hostname"):
			if lab != "_hostname" {
				break
			}
			h, err := os.Hostname()
			if h == "" && err != nil {
				h = err.Error()
			}

			ll.Set("_hostname", h)

			continue
		case strings.HasPrefix(lab, "_pid"):
			if lab != "_pid" {
				break
			}

			ll.Set("_pid", fmt.Sprintf("%d", os.Getpid()))

			continue
		}

		ll = append(ll, lab)
	}

	return ll
}

// New creates new Logger with given writer.
func New(w Writer) *Logger {
	l := &Logger{Writer: w}

	return l
}

// Printf writes logging Message.
// Arguments are handled in the manner of fmt.Printf.
func Printf(f string, args ...interface{}) {
	newmessage(DefaultLogger, Span{}, f, args)
}

// Panicf does the same as Printf but panics in the end.
// panic argument is fmt.Sprintf result with func arguments.
func Panicf(f string, args ...interface{}) {
	newmessage(DefaultLogger, Span{}, f, args)
	panic(fmt.Sprintf(f, args...))
}

// Fatalf does the same as Printf but calls os.Exit(1) in the end.
func Fatalf(f string, args ...interface{}) {
	newmessage(DefaultLogger, Span{}, f, args)
	os.Exit(1)
}

// PrintRaw writes logging Message with given text.
//
// This functions is intended to use in a really hot code.
// All possible allocs are eliminated. You should reuse buffer either.
func PrintRaw(b []byte) {
	newmessage(DefaultLogger, Span{}, bytesToString(b), nil)
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
	if DefaultLogger == nil {
		return nil
	}
	f := (*filter)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&DefaultLogger.filter))))
	if !f.match(tp) {
		return nil
	}
	return DefaultLogger
}

// SetFilter sets filter to use in V.
// Filter is a comma separated list of rules.
// Each rule is one of: topic
//     error
//     networking
//     send
//     encryption
// location
//     path/to/file.go
//     short_file.go
//     path/to/package - subpackages are not selected
//     all/subpackages/* - including root of subpackages
//     github.com/nikandfor/tlog.Function
//     tlog.(*Type).Method
//     tlog.Type - all functions are selected
// topics in location
//     tlog.Span=timing
//     p2p/conn.go=read+write - multiple topics in the location are separated by '+'
//
// SetFilter can be called simultaneously with V.
func SetFilter(f string) {
	DefaultLogger.SetFilter(f)
}

// SetLogLevel is a shortcut for SetFilter with one of *Filter constants
func SetLogLevel(l int) {
	DefaultLogger.SetLogLevel(l)
}

func newspan(l *Logger, par ID) Span {
	var id ID
	for id == 0 {
		id = ID(rnd.Int63())
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

func newmessage(l *Logger, s Span, f string, args []interface{}) {
	if l == nil {
		return
	}

	var t time.Duration
	if s.ID == 0 {
		t = time.Duration(now().UnixNano())
	} else {
		t = now().Sub(s.Started)
	}

	var loc Location
	if !l.NoLocations {
		loc = Caller(2)
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

	return newspan(DefaultLogger, 0)
}

// Spawn creates new child trace.
//
// Trace could be started on one machine and derived on another.
//
// Span must be Finished in the end.
func Spawn(id ID) Span {
	if DefaultLogger == nil || id == 0 {
		return Span{}
	}

	return newspan(DefaultLogger, id)
}

// Panicf writes logging Message to Writer.
// Arguments are handled in the manner of fmt.Printf.
func (l *Logger) Printf(f string, args ...interface{}) {
	newmessage(l, Span{}, f, args)
}

// Panicf writes logging Message and panics.
// Arguments are handled in the manner of fmt.Printf.
func (l *Logger) Panicf(f string, args ...interface{}) {
	newmessage(l, Span{}, f, args)
	panic(fmt.Sprintf(f, args...))
}

// Panicf writes logging Message and calls os.Exit(1) in the end.
// Arguments are handled in the manner of fmt.Printf.
func (l *Logger) Fatalf(f string, args ...interface{}) {
	newmessage(l, Span{}, f, args)
	os.Exit(1)
}

// PrintRaw writes logging Message with given text.
//
// This functions is intended to use in a really hot code.
// All possible allocs are eliminated. You should reuse buffer either.
func (l *Logger) PrintRaw(b []byte) {
	newmessage(l, Span{}, bytesToString(b), nil)
}

// Start creates new root trace.
//
// Span must be Finished in the end.
func (l *Logger) Start() Span {
	if l == nil {
		return Span{}
	}

	return newspan(l, 0)
}

// Spawn creates new child trace.
//
// Trace could be started on one machine and derived on another.
//
// Span must be Finished in the end.
func (l *Logger) Spawn(id ID) Span {
	if l == nil || id == 0 {
		return Span{}
	}

	return newspan(l, id)
}

// V checks if topic tp is enabled and returns default Logger or nil.
//
// It's OK to use nil Logger, it wonn't crash and won't emit eny Messages to writer.
//
// Multiple comma separated topics could be passed. Logger will be non-nil if at least one of these topics is enabled.
func (l *Logger) V(tp string) *Logger {
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

// V checks if span is active (filter condition was true when span was created).
//
// It's quiet similar with checking debug condition as following.
//     if l := Logger.V("topic"); l != nil { /* do complex debug computations only if necessary */ }
func (s Span) V() bool {
	return s.ID != 0
}

// Printf writes logging Message annotated with trace id.
// Arguments are handled in the manner of fmt.Printf.
func (s Span) Printf(f string, args ...interface{}) {
	if s.ID == 0 {
		return
	}

	newmessage(s.l, s, f, args)
}

// PrintRaw writes logging Message with given text annotated with trace id.
//
// This functions is intended to use in a really hot code.
// All possible allocs are eliminated. You should reuse buffer either.
func (s Span) PrintRaw(b []byte) {
	if s.ID == 0 {
		return
	}

	newmessage(s.l, s, bytesToString(b), nil)
}

// Finish writes Span finish event to Writer.
func (s Span) Finish() {
	if s.ID == 0 {
		return
	}

	el := now().Sub(s.Started)
	s.l.SpanFinished(s, el)
}

// Set sets k label value to v
func (ls *Labels) Set(k, v string) {
	val := k
	if v != "" {
		val += "=" + v
	}

	for i := 0; i < len(*ls); i++ {
		l := (*ls)[i]
		if l == "="+k {
			(*ls)[i] = val
			return
		} else if l == k || strings.HasPrefix(l, k+"=") {
			(*ls)[i] = val
			return
		}
	}
	*ls = append(*ls, val)
}

// Get gets k label value or "", false
func (ls *Labels) Get(k string) (string, bool) {
	for _, l := range *ls {
		if l == k {
			return "", true
		} else if strings.HasPrefix(l, k+"=") {
			return l[len(k)+1:], true
		}
	}
	return "", false
}

// Del replaces k label with special thumbstone.
// It's needed because Labels event contains state diff not state itself.
func (ls *Labels) Del(k string) {
	for i := 0; i < len(*ls); i++ {
		l := (*ls)[i]
		if l == "="+k {
			return
		} else if l == k || strings.HasPrefix(l, k+"=") {
			(*ls)[i] = "=" + k
		}
	}
}

// Merge merges two Labels sets
func (ls *Labels) Merge(b Labels) {
	for _, add := range b {
		if add == "" {
			continue
		}
		kv := strings.SplitN(add, "=", 2)
		if kv[0] == "" {
			ls.Del(kv[1])
		} else {
			ls.Set(kv[0], kv[1])
		}
	}
}

// String returns constant width string representation.
func (i ID) String() string {
	if i == 0 {
		return "________________"
	}
	return fmt.Sprintf("%016x", uint64(i))
}

// AbsTime converts Message Time field from nanoseconds from Unix epoch to time.Time
func (m *Message) AbsTime() time.Time {
	return time.Unix(0, int64(m.Time))
}

// Int63 does the same as rand.Int63
func (r *concurrentRand) Int63() int64 {
	defer r.mu.Unlock()
	r.mu.Lock()

	return r.rnd.Int63()
}
