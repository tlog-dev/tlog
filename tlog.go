package tlog

import (
	"fmt"
	"io"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nikandfor/json"
)

type (
	TraceID int64
	SpanID  int64

	FullID struct {
		TraceID
		SpanID
	}

	Logger interface {
		Writer

		Printf(f string, args ...interface{})
		Start() *Span
		Spawn(FullID) *Span
	}

	Writer interface {
		SpanStarted(s *Span)
		SpanFinished(s *Span)
		Message(l *Message, s *Span)
	}

	Message struct {
		Location Location
		Time     time.Duration
		Format   string
		Args     []interface{}
	}

	Span struct {
		l Logger

		ID     FullID
		Parent SpanID

		Location Location

		Start   time.Time
		Elapsed time.Duration

		Logs []*Message

		Flags int

		_ noCopy
	}

	SimpleLogger struct {
		Writer
	}

	ConsoleWriter struct {
		w  io.Writer
		f  int
		tf string
	}

	JSONWriter struct {
		mu sync.Mutex
		w  *json.Writer
		ls map[Location]struct{}
	}

	FilterWriter struct {
		w    Writer
		args []string

		mu sync.RWMutex
		c  map[Location]bool
	}

	TeeWriter struct {
		mu      sync.Mutex
		Writers []Writer
	}

	pos struct {
		File string
		Line int
	}

	Depth int

	noCopy struct{}
)

const (
	FlagError = 1 << iota

	FlagNone = 0
)

var (
	now func() time.Time = time.Now
	rnd                  = rand.New(rand.NewSource(now().UnixNano()))
)

var DefaultLogger Logger = NewLogger(NewConsoleWriter(os.Stderr))

func NewLogger(w Writer) Logger {
	return &SimpleLogger{Writer: w}
}

func Printf(fmt string, args ...interface{}) {
	DefaultLogger.Message(
		&Message{
			Location: location(1),
			Time:     time.Duration(now().UnixNano()),
			Format:   fmt,
			Args:     args,
		},
		nil,
	)
}

func (l *SimpleLogger) Printf(fmt string, args ...interface{}) {
	l.Message(
		&Message{
			Location: location(1),
			Time:     time.Duration(now().UnixNano()),
			Format:   fmt,
			Args:     args,
		},
		nil,
	)
}

func (l *SimpleLogger) Start() *Span {
	s := &Span{
		l:        l,
		ID:       FullID{TraceID(rnd.Int63()), SpanID(rnd.Int63())},
		Location: funcentry(1),
		Start:    now(),
	}
	l.SpanStarted(s)
	return s
}

func (l *SimpleLogger) Spawn(id FullID) *Span {
	s := &Span{
		l:        l,
		ID:       FullID{id.TraceID, SpanID(rnd.Int63())},
		Parent:   id.SpanID,
		Location: funcentry(1),
		Start:    now(),
	}
	l.SpanStarted(s)
	return s
}

func (s *Span) Printf(fmt string, args ...interface{}) {
	s.l.Message(
		&Message{
			Location: location(1),
			Time:     now().Sub(s.Start),
			Format:   fmt,
			Args:     args,
		},
		s,
	)
}

func (s *Span) Finish() {
	s.Elapsed = now().Sub(s.Start)
	s.l.SpanFinished(s)
}

func NewConsoleWriter(w io.Writer) *ConsoleWriter {
	return &ConsoleWriter{
		w:  w,
		tf: "2006-01-02_15:04:05.000000",
	}
}

func (w *ConsoleWriter) Message(m *Message, s *Span) {
	t := time.Unix(0, m.Time.Nanoseconds())
	endl := ""
	if l := len(m.Format); l == 0 || m.Format[l-1] != '\n' {
		endl = "\n"
	}
	fmt.Fprintf(w.w, "%v %-20v "+m.Format+endl, append([]interface{}{t.Format(w.tf), m.Location.String()}, m.Args...)...)
}

func (w *ConsoleWriter) SpanStarted(s *Span) {
	fmt.Fprintf(w.w, "%v %-20v %v Span started\n", s.Start.Format(w.tf), s.Location.String(), s.ID)
}

func (w *ConsoleWriter) SpanFinished(s *Span) {
	fmt.Fprintf(w.w, "%v %-20v %v Span finished - elapsed %v\n", s.Start.Format(w.tf), s.Location.String(), s.ID, s.Elapsed)
}

func (w *FilterWriter) Message(m *Message, s *Span) {
	if !w.should(m.Location, true) {
		return
	}
	w.w.Message(m, s)
}

func (w *FilterWriter) SpanStarted(s *Span) {
	if !w.should(s.Location, false) {
		return
	}
	w.w.SpanStarted(s)
}

func (w *FilterWriter) SpanFinished(s *Span) {
	if !w.should(s.Location, false) {
		return
	}
	w.w.SpanFinished(s)
}

func (w *FilterWriter) should(l Location, msg bool) bool {
	w.mu.RLock()
	r, ok := w.c[l]
	w.mu.RUnlock()
	if ok {
		return r
	}

	defer w.mu.Unlock()
	w.mu.Lock()

	r = w.compile(l, msg)

	w.c[l] = r

	return r
}

func (w *FilterWriter) compile(l Location, msg bool) (r bool) {
	file, _ := l.FileLine()
	//	fnc := l.Func()

	for _, a := range w.args {
		r = strings.Contains(file, a)
	}

	return r
}

func NewJSONWriter(w *json.Writer) *JSONWriter {
	return &JSONWriter{
		w:  w,
		ls: make(map[Location]struct{}),
	}
}

func (w *JSONWriter) Message(m *Message, s *Span) {
	msg := fmt.Sprintf(m.Format, m.Args...)

	defer w.mu.Unlock()
	w.mu.Lock()

	if _, ok := w.ls[m.Location]; !ok {
		w.location(m.Location)
	}

	w.w.ObjStart()

	w.w.ObjKey([]byte("m"))

	w.w.ObjStart()

	w.w.ObjKey([]byte("l"))
	fmt.Fprintf(w.w, "%d", m.Location)

	w.w.ObjKey([]byte("t"))
	fmt.Fprintf(w.w, "%d", m.Time.Nanoseconds()/1000)

	w.w.ObjKey([]byte("m"))
	w.w.StringString(msg)

	if s != nil {
		w.w.ObjKey([]byte("s"))
		fmt.Fprintf(w.w, "%d", s.ID.SpanID)
	}

	w.w.ObjEnd()

	w.w.ObjEnd()

	w.w.NewLine()
}

func (w *JSONWriter) SpanStarted(s *Span) {
	defer w.mu.Unlock()
	w.mu.Lock()

	if _, ok := w.ls[s.Location]; !ok {
		w.location(s.Location)
	}

	w.w.ObjStart()

	w.w.ObjKey([]byte("s"))

	w.w.ObjStart()

	w.w.ObjKey([]byte("tr"))
	fmt.Fprintf(w.w, "%d", s.ID.TraceID)

	w.w.ObjKey([]byte("id"))
	fmt.Fprintf(w.w, "%d", s.ID.SpanID)

	if s.Parent != 0 {
		w.w.ObjKey([]byte("par"))
		w.w.StringString(s.Parent.String())
	}

	w.w.ObjKey([]byte("loc"))
	fmt.Fprintf(w.w, "%d", s.Location)

	w.w.ObjKey([]byte("st"))
	fmt.Fprintf(w.w, "%d", s.Start.UnixNano()/1000)

	w.w.ObjEnd()

	w.w.ObjEnd()

	w.w.NewLine()
}

func (w *JSONWriter) SpanFinished(s *Span) {
	defer w.mu.Unlock()
	w.mu.Lock()

	w.w.ObjStart()

	w.w.ObjKey([]byte("f"))

	w.w.ObjStart()

	w.w.ObjKey([]byte("id"))
	fmt.Fprintf(w.w, "%d", s.ID.SpanID)

	w.w.ObjKey([]byte("el"))
	fmt.Fprintf(w.w, "%d", s.Elapsed.Nanoseconds()/1000)

	if s.Flags != 0 {
		w.w.ObjKey([]byte("F"))
		fmt.Fprintf(w.w, "%x", s.Flags)
	}

	w.w.ObjEnd()

	w.w.ObjEnd()

	w.w.NewLine()
}

func (w *JSONWriter) location(l Location) {
	file, line := l.FileLine()
	fnc := l.Func()

	w.w.ObjStart()

	w.w.ObjKey([]byte("l"))

	w.w.ObjStart()

	w.w.ObjKey([]byte("pc"))
	fmt.Fprintf(w.w, "%d", l)

	w.w.ObjKey([]byte("f"))
	w.w.StringString(file)

	w.w.ObjKey([]byte("l"))
	fmt.Fprintf(w.w, "%d", line)

	w.w.ObjKey([]byte("n"))
	w.w.StringString(fnc)

	w.w.ObjEnd()

	w.w.ObjEnd()

	w.w.NewLine()

	w.ls[l] = struct{}{}
}

func NewTeeWriter(w ...Writer) TeeWriter {
	return TeeWriter{Writers: w}
}

func (w TeeWriter) Message(m *Message, s *Span) {
	defer w.mu.Unlock()
	w.mu.Lock()

	for _, w := range w.Writers {
		w.Message(m, s)
	}
}

func (w TeeWriter) SpanStarted(s *Span) {
	defer w.mu.Unlock()
	w.mu.Lock()

	for _, w := range w.Writers {
		w.SpanStarted(s)
	}
}

func (w TeeWriter) SpanFinished(s *Span) {
	defer w.mu.Unlock()
	w.mu.Lock()

	for _, w := range w.Writers {
		w.SpanFinished(s)
	}
}

func (i TraceID) String() string {
	return fmt.Sprintf("%016x", uint64(i))
}

func (i SpanID) String() string {
	return fmt.Sprintf("%016x", uint64(i))
}

func (i FullID) String() string {
	return i.TraceID.String() + ":" + i.SpanID.String()
}

func (*noCopy) Lock() {}
