package tlog

import (
	"fmt"
	"io"
	"math/rand"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nikandfor/json"
)

type (
	ID int64

	Labels []string

	Logger interface {
		Writer

		Printf(f string, args ...interface{})
		Start() *Span
		Spawn(ID) *Span
	}

	Writer interface {
		Labels(ls Labels)
		SpanStarted(s *Span)
		SpanFinished(s *Span)
		Message(l Message, s *Span)
	}

	Message struct {
		Location Location
		Time     time.Duration
		Format   string
		Args     []interface{}
	}

	Span struct {
		l Logger

		ID     ID
		Parent ID

		Location Location

		Started time.Time
		Elapsed time.Duration

		Flags int
	}

	SimpleLogger struct {
		Writer
	}

	initLogger struct{}

	ConsoleWriter struct {
		mu  sync.Mutex
		lb  lastByte
		f   int
		buf []byte
		nl  []byte
	}

	JSONWriter struct {
		mu  sync.Mutex
		w   *json.Writer
		ls  map[Location]struct{}
		buf []byte
	}

	FilterWriter struct {
		w Writer

		mu sync.RWMutex
		c  map[Location]bool
	}

	TeeWriter struct {
		mu      sync.Mutex
		Writers []Writer
	}

	Rand interface {
		Int63() int64
	}

	ConcurrentRand struct {
		mu  sync.Mutex
		rnd Rand
	}

	lastByte struct {
		w io.Writer
		l byte
	}
)

const (
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
	LUTC
	Lspans
	LstdFlags = Ldate | Ltime
	LdetFlags = Ldate | Ltime | Lmicroseconds | Lshortfile
)

var ( // time, rand
	now      = time.Now
	rnd Rand = &ConcurrentRand{rnd: rand.New(rand.NewSource(now().UnixNano()))}

	digits = []byte("0123456789abcdef")
)

var ( // defaults
	DefaultLabels   Labels
	DefaultLogger   Logger = initLogger{}
	DumpDefaultInfo        = true
)

func init() {
	h, err := os.Hostname()
	if h == "" && err != nil {
		h = err.Error()
	}

	DefaultLabels = Labels{
		"_hostname=" + h,
		fmt.Sprintf("_pid=%d", os.Getpid()),
	}
}

func NewLogger(w Writer) Logger {
	l := &SimpleLogger{Writer: w}
	if DumpDefaultInfo {
		l.Labels(DefaultLabels)
		l.Printf("!os.Args: %q", os.Args)
	}

	return l
}

func Printf(f string, args ...interface{}) {
	newmessage(DefaultLogger, nil, f, args)
}

func newspan(l Logger, par ID) *Span {
	s := &Span{
		l:        l,
		ID:       ID(rnd.Int63()),
		Parent:   par,
		Location: funcentry(2),
		Started:  now(),
	}
	l.SpanStarted(s)
	return s
}

func newmessage(l Logger, s *Span, f string, args []interface{}) {
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
			Location: location(2),
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

func (l *SimpleLogger) Printf(f string, args ...interface{}) {
	newmessage(l, nil, f, args)
}

func (l *SimpleLogger) Start() *Span {
	if l == nil {
		return nil
	}

	return newspan(l, 0)
}

func (l *SimpleLogger) Spawn(id ID) *Span {
	if l == nil || id == 0 {
		return nil
	}

	return newspan(l, id)
}

func (l initLogger) Labels(ls Labels)           { l.init(); DefaultLogger.Labels(ls) }
func (l initLogger) Message(m Message, s *Span) { l.init(); DefaultLogger.Message(m, s) }
func (l initLogger) SpanStarted(s *Span)        { l.init(); DefaultLogger.SpanStarted(s) }
func (l initLogger) SpanFinished(s *Span)       { l.init(); DefaultLogger.SpanFinished(s) }

func (l initLogger) Printf(f string, args ...interface{}) {
	l.init()
	newmessage(DefaultLogger, nil, f, args)
}

func (l initLogger) Start() *Span {
	l.init()

	return newspan(DefaultLogger, 0)
}

func (l initLogger) Spawn(id ID) *Span {
	l.init()

	return newspan(DefaultLogger, id)
}

func (l initLogger) init() {
	DefaultLogger = NewLogger(NewConsoleWriter(os.Stderr, LstdFlags))
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

	s.Elapsed = now().Sub(s.Started)
	s.l.SpanFinished(s)
}

func NewConsoleWriter(w io.Writer, f int) *ConsoleWriter {
	return &ConsoleWriter{
		lb: lastByte{w: w},
		f:  f,
		nl: []byte{'\n'},
	}
}

func (w *ConsoleWriter) buildHeader(t time.Time, loc Location) {
	b := w.buf
	b = b[:cap(b)]
	i := 0

	if w.f&(Ldate|Ltime|Lmicroseconds) != 0 {
		if w.f&LUTC != 0 {
			t = t.UTC()
		}
		if w.f&Ldate != 0 {
			if len(b[i:]) < 15 {
				b = append(b,
					0, 0, 0, 0, 0,
					0, 0, 0, 0, 0,
					0, 0, 0, 0, 0)
			}

			y, m, d := t.Date()
			for j := 3; j >= 0; j-- {
				b[i+j] = '0' + byte(y%10)
				y /= 10
			}
			i += 4

			b[i] = '/'
			i++

			b[i] = '0' + byte(m/10)
			i++
			b[i] = '0' + byte(m%10)
			i++

			b[i] = '/'
			i++

			b[i] = '0' + byte(d/10)
			i++
			b[i] = '0' + byte(d%10)
			i++
		}
		if w.f&Ltime != 0 {
			if len(b[i:]) < 12 {
				b = append(b,
					0, 0, 0, 0, 0,
					0, 0, 0, 0, 0,
					0, 0)
			}

			if i != 0 {
				b[i] = '_'
				i++
			}
			h, m, s := t.Clock()

			b[i] = '0' + byte(h/10)
			i++
			b[i] = '0' + byte(h%10)
			i++

			b[i] = ':'
			i++

			b[i] = '0' + byte(m/10)
			i++
			b[i] = '0' + byte(m%10)
			i++

			b[i] = ':'
			i++

			b[i] = '0' + byte(s/10)
			i++
			b[i] = '0' + byte(s%10)
			i++
		}
		if w.f&(Lmilliseconds|Lmicroseconds) != 0 {
			if len(b[i:]) < 10 {
				b = append(b, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
			}
			if i != 0 {
				b[i] = '.'
				i++
			}
			ns := t.Nanosecond() / 1e3
			n := 6
			if w.f&Lmicroseconds == 0 {
				n = 3
				ns /= 1e3
			}
			for j := n - 1; j >= 0; j-- {
				b[i+j] = '0' + byte(ns%10)
				ns /= 10
			}
			i += n
		}
		b[i] = ' '
		i++
		b[i] = ' '
		i++
	}
	if w.f&(Llongfile|Lshortfile) != 0 {
		W := i + 20
		if len(b) < W+5 {
			b = append(b,
				0, 0, 0, 0, 0,
				0, 0, 0, 0, 0,
				0, 0, 0, 0, 0,
				0, 0, 0, 0, 0,
				0, 0, 0, 0, 0,
				0, 0, 0, 0, 0,
			)
		}

		_, f, l := loc.NameFileLine()
		if w.f&Lshortfile != 0 {
			f = path.Base(f)
		}
		b = append(b[:i], f...)
		i += len(f)
		if len(b[i:]) < 10 {
			b = append(b, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
		}

		b[i] = ':'
		i++

		j := 0
		for q := 10; q < l; q *= 10 {
			j++
		}
		n := j + 1
		for ; j >= 0; j-- {
			b[i+j] = '0' + byte(l%10)
			l /= 10
		}
		i += n

		b = b[:cap(b)]

		for i < W {
			b[i] = ' '
			i++
		}

		b[i] = ' '
		i++
		b[i] = ' '
		i++
	}

	w.buf = b[:i]
}

func (w *ConsoleWriter) Message(m Message, s *Span) {
	defer w.mu.Unlock()
	w.mu.Lock()

	var t time.Time
	if s != nil {
		t = s.Started.Add(m.Time)
	} else {
		t = m.AbsTime()
	}

	w.buildHeader(t, m.Location)

	_, _ = w.lb.w.Write(w.buf)

	_, _ = fmt.Fprintf(&w.lb, m.Format, m.Args...)

	if w.lb.l != '\n' {
		_, _ = w.lb.w.Write(w.nl)
	}
}

func (w *ConsoleWriter) SpanStarted(s *Span) {
	if w.f&(Lspans) == 0 {
		return
	}

	defer w.mu.Unlock()
	w.mu.Lock()

	w.buildHeader(s.Started, s.Location)

	b := w.buf

	b = append(b, "!Span started  "...)
	i := len(b)
	if cap(b) >= i+17 {
		b = b[:cap(b)]
	} else {
		b = append(b, 0, 0,
			0, 0, 0, 0, 0,
			0, 0, 0, 0, 0,
			0, 0, 0, 0, 0)
	}

	id := s.ID
	for j := 15; j >= 0; j-- {
		b[i+j] = digits[id&0x7]
		id >>= 4
	}
	i += 16
	b[i] = '\n'
	i++
	w.buf = b[:i]

	_, _ = w.lb.w.Write(w.buf)
}

func (w *ConsoleWriter) SpanFinished(s *Span) {
	if w.f&(Lspans) == 0 {
		return
	}

	defer w.mu.Unlock()
	w.mu.Lock()

	w.buildHeader(s.Started, s.Location)

	b := w.buf

	b = append(b, "!Span finished "...)
	i := len(b)
	if cap(b) >= i+16 {
		b = b[:cap(b)]
	} else {
		b = append(b, 0, 0,
			0, 0, 0, 0, 0,
			0, 0, 0, 0, 0,
			0, 0, 0, 0, 0)
	}

	id := s.ID
	for j := 15; j >= 0; j-- {
		b[i+j] = digits[id&0xf]
		id >>= 4
	}
	i += 16

	b = append(b[:i], " - elapsed "...)

	e := s.Elapsed.Seconds()
	suff := "s"
	if s.Elapsed < time.Second {
		e *= 1000
		suff = "ms"
	}

	pr := 2
	b = strconv.AppendFloat(b, e, 'f', pr, 64)

	b = append(b, suff...)
	b = append(b, '\n')

	w.buf = b

	_, _ = w.lb.w.Write(w.buf)
}

func (w *ConsoleWriter) Labels(ls Labels) {
	w.Message(
		Message{
			Location: location(1),
			Time:     time.Duration(now().UnixNano()),
			Format:   "!Labels: %q",
			Args:     []interface{}{ls},
		},
		nil,
	)
}

func (w *FilterWriter) Message(m Message, s *Span) {
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
	return false
}

func NewJSONWriter(w *json.Writer) *JSONWriter {
	return &JSONWriter{
		w:  w,
		ls: make(map[Location]struct{}),
	}
}

func (w *JSONWriter) Labels(ls Labels) {
	defer w.mu.Unlock()
	w.mu.Lock()

	w.w.ObjStart()

	w.w.ObjKey([]byte("L"))

	w.w.ArrayStart()

	for _, l := range ls {
		w.w.SafeStringString(l)
	}

	w.w.ArrayEnd()

	w.w.ObjEnd()

	w.w.NewLine()
}

func (w *JSONWriter) Message(m Message, s *Span) {
	defer w.mu.Unlock()
	w.mu.Lock()

	if _, ok := w.ls[m.Location]; !ok {
		w.location(m.Location)
	}

	b := w.buf

	w.w.ObjStart()

	w.w.ObjKey([]byte("m"))

	w.w.ObjStart()

	w.w.ObjKey([]byte("l"))
	b = strconv.AppendInt(b[:0], int64(m.Location), 10)
	_, _ = w.w.Write(b)

	w.w.ObjKey([]byte("t"))
	b = strconv.AppendInt(b[:0], m.Time.Nanoseconds()/1000, 10)
	_, _ = w.w.Write(b)

	w.w.ObjKey([]byte("m"))
	sw := w.w.StringWriter()
	_, _ = fmt.Fprintf(sw, m.Format, m.Args...)
	sw.Close()

	if s != nil {
		w.w.ObjKey([]byte("s"))
		b = strconv.AppendInt(b[:0], int64(s.ID), 10)
		_, _ = w.w.Write(b)
	}

	w.w.ObjEnd()

	w.w.ObjEnd()

	w.w.NewLine()

	w.buf = b
}

func (w *JSONWriter) SpanStarted(s *Span) {
	defer w.mu.Unlock()
	w.mu.Lock()

	if _, ok := w.ls[s.Location]; !ok {
		w.location(s.Location)
	}

	b := w.buf

	w.w.ObjStart()

	w.w.ObjKey([]byte("s"))

	w.w.ObjStart()

	w.w.ObjKey([]byte("id"))
	b = strconv.AppendInt(b[:0], int64(s.ID), 10)
	_, _ = w.w.Write(b)

	if s.Parent != 0 {
		w.w.ObjKey([]byte("p"))
		b = strconv.AppendInt(b[:0], int64(s.Parent), 10)
		_, _ = w.w.Write(b)
	}

	w.w.ObjKey([]byte("l"))
	b = strconv.AppendInt(b[:0], int64(s.Location), 10)
	_, _ = w.w.Write(b)

	w.w.ObjKey([]byte("s"))
	b = strconv.AppendInt(b[:0], s.Started.UnixNano()/1000, 10)
	_, _ = w.w.Write(b)

	w.w.ObjEnd()

	w.w.ObjEnd()

	w.w.NewLine()

	w.buf = b
}

func (w *JSONWriter) SpanFinished(s *Span) {
	defer w.mu.Unlock()
	w.mu.Lock()

	b := w.buf

	w.w.ObjStart()

	w.w.ObjKey([]byte("f"))

	w.w.ObjStart()

	w.w.ObjKey([]byte("id"))
	b = strconv.AppendInt(b[:0], int64(s.ID), 10)
	_, _ = w.w.Write(b)

	w.w.ObjKey([]byte("e"))
	b = strconv.AppendInt(b[:0], s.Elapsed.Nanoseconds()/1000, 10)
	_, _ = w.w.Write(b)

	if s.Flags != 0 {
		w.w.ObjKey([]byte("F"))
		b = strconv.AppendInt(b[:0], int64(s.Flags), 10)
		_, _ = w.w.Write(b)
	}

	w.w.ObjEnd()

	w.w.ObjEnd()

	w.w.NewLine()

	w.buf = b
}

func (w *JSONWriter) location(l Location) {
	name, file, line := l.NameFileLine()
	name = path.Base(name)

	b := w.buf

	w.w.ObjStart()

	w.w.ObjKey([]byte("l"))

	w.w.ObjStart()

	w.w.ObjKey([]byte("pc"))
	b = strconv.AppendInt(b[:0], int64(l), 10)
	_, _ = w.w.Write(b)

	w.w.ObjKey([]byte("f"))
	w.w.SafeStringString(file)

	w.w.ObjKey([]byte("l"))
	b = strconv.AppendInt(b[:0], int64(line), 10)
	_, _ = w.w.Write(b)

	w.w.ObjKey([]byte("n"))
	w.w.StringString(name)

	w.w.ObjEnd()

	w.w.ObjEnd()

	w.w.NewLine()

	w.ls[l] = struct{}{}
	w.buf = b
}

func NewTeeWriter(w ...Writer) *TeeWriter {
	return &TeeWriter{Writers: w}
}

func (w *TeeWriter) Labels(ls Labels) {
	defer w.mu.Unlock()
	w.mu.Lock()

	for _, w := range w.Writers {
		w.Labels(ls)
	}
}

func (w *TeeWriter) Message(m Message, s *Span) {
	defer w.mu.Unlock()
	w.mu.Lock()

	for _, w := range w.Writers {
		w.Message(m, s)
	}
}

func (w *TeeWriter) SpanStarted(s *Span) {
	defer w.mu.Unlock()
	w.mu.Lock()

	for _, w := range w.Writers {
		w.SpanStarted(s)
	}
}

func (w *TeeWriter) SpanFinished(s *Span) {
	defer w.mu.Unlock()
	w.mu.Lock()

	for _, w := range w.Writers {
		w.SpanFinished(s)
	}
}

func (ls *Labels) Set(k, v string) {
	val := k
	if v != "" {
		val += "=" + v
	}

	for i, l := range *ls {
		if l == k || strings.HasPrefix(l, k+"=") {
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

func (ls *Labels) Del(k string) bool {
	for i, l := range *ls {
		if l == k || strings.HasPrefix(l, k+"=") {
			ll := len(*ls) - 1
			(*ls)[i] = (*ls)[ll]
			(*ls) = (*ls)[:ll]
			return true
		}
	}
	return false
}

func (ls *Labels) Merge(b Labels) {
out:
	for _, add := range b {
		for _, have := range *ls {
			if add == have {
				continue out
			}
		}

		*ls = append(*ls, add)
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

func (r *ConcurrentRand) Int63() int64 {
	defer r.mu.Unlock()
	r.mu.Lock()

	return r.rnd.Int63()
}

func (w *lastByte) Write(p []byte) (int, error) {
	l := len(p)
	if l == 0 {
		return 0, nil
	}
	w.l = p[l-1]
	return w.w.Write(p)
}
