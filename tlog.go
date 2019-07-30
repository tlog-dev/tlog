// tlog is an logger and a tracer in the one package.
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
	ID int64

	Labels []string

	Logger struct {
		Writer
		filter *filter
	}

	Writer interface {
		Labels(ls Labels)
		SpanStarted(s *Span, l Location)
		SpanFinished(s *Span, el time.Duration)
		Message(l Message, s *Span)
	}

	Message struct {
		Location Location
		Time     time.Duration
		Format   string
		Args     []interface{}
	}

	Span struct {
		l *Logger

		ID     ID
		Parent ID

		Started time.Time

		Flags int
	}

	Rand interface {
		Int63() int64
	}

	concurrentRand struct {
		mu  sync.Mutex
		rnd Rand
	}
)

const ( // span flags
	FlagError = 1 << iota

	FlagNone = 0
)

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
	Lspans       // print Span start and finish event
	Lmessagespan // add Span ID to trace messages
	LstdFlags    = Ldate | Ltime
	LdetFlags    = Ldate | Ltime | Lmicroseconds | Lshortfile
	Lnone        = 0
)

const ( // log levels
	CriticalLevel = "critical"
	ErrorLevel    = "error"
	InfoLevel     = "info"
	DebugLevel    = "debug"
	TraceLevel    = "trace"

	CriticalFilter = CriticalLevel
	ErrorFilter    = CriticalFilter + "+" + ErrorLevel
	InfoFilter     = ErrorFilter + "+" + InfoLevel
	DebugFilter    = InfoFilter + "+" + DebugLevel
	TraceFilter    = DebugFilter + "+" + TraceLevel
)

var ( // time, rand
	now      = time.Now
	rnd Rand = &concurrentRand{rnd: rand.New(rand.NewSource(now().UnixNano()))}

	digits = []byte("0123456789abcdef")
)

var ( // defaults
	DefaultLogger = New(NewConsoleWriter(os.Stderr, LstdFlags))
)

func FillLabelsWithDefaults(labels ...string) Labels {
	var ll Labels

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

func New(w Writer) *Logger {
	l := &Logger{Writer: w}

	return l
}

func Printf(f string, args ...interface{}) {
	newmessage(DefaultLogger, nil, f, args)
}

func Panicf(f string, args ...interface{}) {
	newmessage(DefaultLogger, nil, f, args)
	panic(fmt.Sprintf(f, args...))
}

func Fatalf(f string, args ...interface{}) {
	newmessage(DefaultLogger, nil, f, args)
	os.Exit(1)
}

func V(topic string) *Logger {
	return DefaultLogger.V(topic)
}

func SetFilter(f string) {
	DefaultLogger.SetFilter(f)
}

func SetLogLevel(l int) {
	DefaultLogger.SetLogLevel(l)
}

func newspan(l *Logger, par ID) *Span {
	loc := Funcentry(2)
	s := &Span{
		l:       l,
		ID:      ID(rnd.Int63()),
		Parent:  par,
		Started: now(),
	}
	l.SpanStarted(s, loc)
	return s
}

func newmessage(l *Logger, s *Span, f string, args []interface{}) {
	if l == nil {
		return
	}

	var t time.Duration
	if s == nil {
		t = time.Duration(now().UnixNano())
	} else {
		t = now().Sub(s.Started)
	}

	l.Message(
		Message{
			Location: Caller(2),
			Time:     t,
			Format:   f,
			Args:     args,
		},
		s,
	)
}

func Start() *Span {
	if DefaultLogger == nil {
		return nil
	}

	return newspan(DefaultLogger, 0)
}

func Spawn(id ID) *Span {
	if DefaultLogger == nil || id == 0 {
		return nil
	}

	return newspan(DefaultLogger, id)
}

func (l *Logger) Printf(f string, args ...interface{}) {
	newmessage(l, nil, f, args)
}

func (l *Logger) Panicf(f string, args ...interface{}) {
	newmessage(l, nil, f, args)
	panic(fmt.Sprintf(f, args...))
}

func (l *Logger) Fatalf(f string, args ...interface{}) {
	newmessage(l, nil, f, args)
	os.Exit(1)
}

func (l *Logger) Start() *Span {
	if l == nil {
		return nil
	}

	return newspan(l, 0)
}

func (l *Logger) Spawn(id ID) *Span {
	if l == nil || id == 0 {
		return nil
	}

	return newspan(l, id)
}

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

func (l *Logger) SetLogLevel(lev int) {
	switch {
	case lev <= 0:
		l.SetFilter("")
	case lev == 1:
		l.SetFilter(CriticalFilter)
	case lev == 2:
		l.SetFilter(ErrorFilter)
	case lev == 3:
		l.SetFilter(InfoFilter)
	case lev == 4:
		l.SetFilter(DebugFilter)
	default:
		l.SetFilter(TraceFilter)
	}
}

func (s *Span) Printf(f string, args ...interface{}) {
	if s == nil {
		return
	}

	newmessage(s.l, s, f, args)
}

func (s *Span) Finish() {
	if s == nil {
		return
	}

	el := now().Sub(s.Started)
	s.l.SpanFinished(s, el)
}

func (s *Span) SafeID() ID {
	if s == nil {
		return 0
	}
	return s.ID
}

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

func (i ID) String() string {
	if i == 0 {
		return "________________"
	}
	return fmt.Sprintf("%016x", uint64(i))
}

func (m *Message) AbsTime() time.Time {
	return time.Unix(0, int64(m.Time))
}

func (m *Message) SpanID() ID {
	if m == nil || m.Args == nil {
		return 0
	}
	return m.Args[0].(ID)
}

func (r *concurrentRand) Int63() int64 {
	defer r.mu.Unlock()
	r.mu.Lock()

	return r.rnd.Int63()
}
