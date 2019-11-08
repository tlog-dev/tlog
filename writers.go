package tlog

import (
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nikandfor/json"
)

//go:generate protoc --go_out=. tlogpb/tlog.proto

type (
	// ConsoleWriter produces similar output as stdlib log.Logger.
	//
	// Each event ends up with a single Write.
	//
	// It's safe to write event simultaneously.
	ConsoleWriter struct {
		mu        sync.Mutex
		w         io.Writer
		f         int
		shortfile int
		funcname  int
		buf       bufWriter
	}

	// JSONWriter produces output readable by both machines and humans.
	//
	// Each event ends up with a single Write if message fits in 1000 bytes (default) buffer.
	//
	// It's safe to write event simultaneously.
	//
	// It's not recommended to use buffered io.Writer because you'll loose last messages in case of crash.
	JSONWriter struct {
		mu  sync.Mutex
		w   *json.Writer
		ls  map[Location]struct{}
		buf []byte
	}

	// ProtoWriter encodes event logs in protobuf and produces more compact output then JSONWriter.
	//
	// Each event ends up with a single Write.
	//
	// It's safe to write event simultaneously.
	//
	// It's not recommended to use buffered io.Writer because you'll loose last messages in case of crash.
	ProtoWriter struct {
		mu  sync.Mutex
		w   io.Writer
		ls  map[Location]struct{}
		buf bufWriter
	}

	// TeeWriter writes the same events in the same order to all Writers one after another.
	TeeWriter struct {
		mu      sync.Mutex
		Writers []Writer
	}

	// Discard discards all events.
	Discard struct{}

	bufWriter []byte
)

// NewConsoleWriter creates writer with similar output as log.Logger.
func NewConsoleWriter(w io.Writer, f int) *ConsoleWriter {
	return &ConsoleWriter{
		w:         w,
		f:         f,
		shortfile: 20,
		funcname:  17,
	}
}

func grow(b []byte, l int) []byte {
more:
	b = b[:cap(b)]
	if len(b) >= l {
		return b
	}

	b = append(b,
		0, 0, 0, 0, 0,
		0, 0, 0, 0, 0,
		0, 0, 0, 0, 0,
		0, 0, 0, 0, 0)

	goto more
}

func (w *ConsoleWriter) appendSegments(b []byte, i, wid int, name string, s byte) ([]byte, int) {
	b = grow(b, i+wid+5)
	W := i + wid

	if nl := len(name); nl <= wid {
		i += copy(b[i:], name)
		for j := i; j < W; j++ {
			b[j] = ' '
		}
		return b, i
	}

	for i+2 < W {
		if len(name) <= W-i {
			i += copy(b[i:], name)
			break
		}

		p := strings.IndexByte(name, s)
		if p == -1 {
			i += copy(b[i:], name[:W-i])
			break
		}

		if len(name)-p < W-i {
			copy(b[i:], name)
			i = W - (len(name) - p)

			b[i] = s
			i++
		} else {
			b[i] = name[0]
			i++
			b[i] = s
			i++
		}

		name = name[p+1:]
	}

	return b, i
}

func (w *ConsoleWriter) buildHeader(loc Location, t time.Time) {
	b := w.buf
	b = b[:cap(b)]
	i := 0

	var fname, file string
	var line = -1

	if w.f&(Ldate|Ltime|Lmilliseconds|Lmicroseconds) != 0 {
		if w.f&LUTC != 0 {
			t = t.UTC()
		}
		if w.f&Ldate != 0 {
			b = grow(b, i+15)

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
			b = grow(b, i+12)

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
			b = grow(b, i+12)

			if i != 0 {
				b[i] = '.'
				i++
			}

			ns := t.Nanosecond() / 1e3
			n := 6
			if w.f&Lmilliseconds != 0 {
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
		fname, file, line = loc.NameFileLine()

		if w.f&Lshortfile != 0 {
			file = filepath.Base(file)
		}

		j := 0
		for q := 10; q < line; q *= 10 {
			j++
		}
		n := 1 + j

		var st int
		if w.f&Lshortfile != 0 {
			b, st = w.appendSegments(b, i, w.shortfile-n-1, file, '/')
		} else {
			b = append(b[:i], file...)
			i += len(file)
			st = i
		}

		b = grow(b, st+10)

		b[st] = ':'
		st++

		for ; j >= 0; j-- {
			b[st+j] = '0' + byte(line%10)
			line /= 10
		}
		st += n

		if w.f&Lshortfile != 0 {
			W := i + w.shortfile
			for ; st < W; st++ {
				b[st] = ' '
			}
			i += w.shortfile
		} else {
			i = st
		}

		b[i] = ' '
		i++
		b[i] = ' '
		i++
	}
	if w.f&(Ltypefunc|Lfuncname) != 0 {
		if line == -1 {
			fname, _, _ = loc.NameFileLine()
		}
		fname = filepath.Base(fname)

		if w.f&Lfuncname != 0 {
			p := strings.Index(fname, ").")
			if p == -1 {
				p = strings.IndexByte(fname, '.')
				fname = fname[p+1:]
			} else {
				fname = fname[p+2:]
			}

			if l := len(fname); l <= w.funcname {
				W := i + w.funcname
				b = grow(b, W+4)
				i += copy(b[i:], fname)
				for ; i < W; i++ {
					b[i] = ' '
				}
			} else {
				i += copy(b[i:], fname[:w.funcname])
				j := 1
				for {
					q := fname[l-j]
					if q < '0' || '9' < q {
						break
					}
					b[i-j] = fname[l-j]
					j++
				}
			}
		} else {
			b = append(b[:i], fname...)
			i += len(fname)
		}

		b = grow(b, i+4)

		b[i] = ' '
		i++
		b[i] = ' '
		i++
	}

	w.buf = b[:i]
}

// Message writes Message event by single Write.
func (w *ConsoleWriter) Message(m Message, s Span) {
	defer w.mu.Unlock()
	w.mu.Lock()

	var t time.Time
	if s.ID != 0 {
		t = s.Started.Add(m.Time)
	} else {
		t = m.AbsTime()
	}

	w.buildHeader(m.Location, t)

	if s.ID != 0 && w.f&Lmessagespan != 0 {
		b := append(w.buf, "Span "...)
		i := len(b)
		b = grow(b, i+20)

		id := s.ID
		for j := 15; j >= 0; j-- {
			b[i+j] = digits[id&0xf]
			id >>= 4
		}
		i += 16

		b[i] = ' '
		i++

		w.buf = b[:i]
	}

	_, _ = fmt.Fprintf(&w.buf, m.Format, m.Args...)

	w.buf.NewLine()

	_, _ = w.w.Write(w.buf)
}

func (w *ConsoleWriter) spanHeader(sid, par ID, loc Location, tm time.Time) []byte {
	w.buildHeader(loc, tm)

	b := w.buf

	b = append(b, "Span "...)
	i := len(b)
	b = b[:i]

	b = grow(b, i+40)

	id := sid
	for j := 15; j >= 0; j-- {
		b[i+j] = digits[id&0xf]
		id >>= 4
	}
	i += 16

	if loc != 0 {
		i += copy(b[i:], " par ")

		id = par
		if id == 0 {
			for j := 15; j >= 0; j-- {
				b[i+j] = '_'
			}
		} else {
			for j := 15; j >= 0; j-- {
				b[i+j] = digits[id&0xf]
				id >>= 4
			}
		}
		i += 16
	}

	b[i] = ' '
	i++
	//	b[i] = ' '
	//	i++

	b = b[:i]

	//	b = append(b, loc...)

	return b
}

// Message writes SpanStarted event by single Write.
func (w *ConsoleWriter) SpanStarted(s Span, par ID, l Location) {
	if w.f&Lspans == 0 {
		return
	}

	defer w.mu.Unlock()
	w.mu.Lock()

	b := w.spanHeader(s.ID, par, l, s.Started)

	b = append(b, "started\n"...)

	w.buf = b

	_, _ = w.w.Write(b)
}

// Message writes SpanFinished event by single Write.
func (w *ConsoleWriter) SpanFinished(s Span, el time.Duration) {
	if w.f&Lspans == 0 {
		return
	}

	defer w.mu.Unlock()
	w.mu.Lock()

	b := w.spanHeader(s.ID, 0, 0, s.Started.Add(el))

	b = append(b, "finished - elapsed "...)

	e := el.Seconds() * 1000

	b = strconv.AppendFloat(b, e, 'f', 2, 64)
	b = append(b, "ms\n"...)

	w.buf = b

	_, _ = w.w.Write(b)
}

// Message writes Labels by single Write.
func (w *ConsoleWriter) Labels(ls Labels) {
	w.Message(
		Message{
			Location: Caller(1),
			Time:     time.Duration(now().UnixNano()),
			Format:   "Labels: %q",
			Args:     []interface{}{ls},
		},
		Span{},
	)
}

// NewConsoleWriter creates JSON writer.
//
// It's not recommended to use buffered io.Writer because you'll loose last messages in case of crash.
func NewJSONWriter(w io.Writer) *JSONWriter {
	return NewCustomJSONWriter(json.NewStreamWriter(w))
}

// NewCustomJSONWriter creates writer with similar output as log.Logger.
//
// It's not recommended to use buffered io.Writer because you'll loose last messages in case of crash.
//
// json.Writer has buffer internally but it's Flushed after each event.
func NewCustomJSONWriter(w *json.Writer) *JSONWriter {
	return &JSONWriter{
		w:  w,
		ls: make(map[Location]struct{}),
	}
}

// Labels writes Labels to the stream.
func (w *JSONWriter) Labels(ls Labels) {
	defer w.mu.Unlock()
	defer w.w.Flush()
	w.mu.Lock()

	w.w.ObjStart()

	w.w.ObjKey([]byte("L"))

	w.w.ArrayStart()

	for _, l := range ls {
		w.w.StringString(l)
	}

	w.w.ArrayEnd()

	w.w.ObjEnd()

	w.w.NewLine()
}

// Message writes event to the stream.
func (w *JSONWriter) Message(m Message, s Span) {
	defer w.mu.Unlock()
	defer w.w.Flush()
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

	if s.ID != 0 {
		w.w.ObjKey([]byte("s"))
		b = strconv.AppendInt(b[:0], int64(s.ID), 10)
		_, _ = w.w.Write(b)
	}

	w.w.ObjEnd()

	w.w.ObjEnd()

	w.w.NewLine()

	w.buf = b
}

// SpanStarted writes event to the stream.
func (w *JSONWriter) SpanStarted(s Span, par ID, loc Location) {
	defer w.mu.Unlock()
	defer w.w.Flush()
	w.mu.Lock()

	if _, ok := w.ls[loc]; !ok {
		w.location(loc)
	}

	b := w.buf

	w.w.ObjStart()

	w.w.ObjKey([]byte("s"))

	w.w.ObjStart()

	w.w.ObjKey([]byte("id"))
	b = strconv.AppendInt(b[:0], int64(s.ID), 10)
	_, _ = w.w.Write(b)

	if par != 0 {
		w.w.ObjKey([]byte("p"))
		b = strconv.AppendInt(b[:0], int64(par), 10)
		_, _ = w.w.Write(b)
	}

	w.w.ObjKey([]byte("l"))
	b = strconv.AppendInt(b[:0], int64(loc), 10)
	_, _ = w.w.Write(b)

	w.w.ObjKey([]byte("s"))
	b = strconv.AppendInt(b[:0], s.Started.UnixNano()/1000, 10)
	_, _ = w.w.Write(b)

	w.w.ObjEnd()

	w.w.ObjEnd()

	w.w.NewLine()

	w.buf = b
}

// SpanFinished writes event to the stream.
func (w *JSONWriter) SpanFinished(s Span, el time.Duration) {
	defer w.mu.Unlock()
	defer w.w.Flush()
	w.mu.Lock()

	b := w.buf

	w.w.ObjStart()

	w.w.ObjKey([]byte("f"))

	w.w.ObjStart()

	w.w.ObjKey([]byte("id"))
	b = strconv.AppendInt(b[:0], int64(s.ID), 10)
	_, _ = w.w.Write(b)

	w.w.ObjKey([]byte("e"))
	b = strconv.AppendInt(b[:0], el.Nanoseconds()/1000, 10)
	_, _ = w.w.Write(b)

	w.w.ObjEnd()

	w.w.ObjEnd()

	w.w.NewLine()

	w.buf = b
}

func (w *JSONWriter) location(l Location) {
	name, file, line := l.NameFileLine()
	//	name = path.Base(name)

	b := w.buf

	w.w.ObjStart()

	w.w.ObjKey([]byte("l"))

	w.w.ObjStart()

	w.w.ObjKey([]byte("pc"))
	b = strconv.AppendInt(b[:0], int64(l), 10)
	_, _ = w.w.Write(b)

	w.w.ObjKey([]byte("f"))
	w.w.StringString(file)

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

// NewConsoleWriter creates Protobuf writer.
//
// It's not recommended to use buffered io.Writer because you'll loose last messages in case of crash.
func NewProtoWriter(w io.Writer) *ProtoWriter {
	return &ProtoWriter{
		w:  w,
		ls: make(map[Location]struct{}),
	}
}

// Labels writes Labels to the stream.
func (w *ProtoWriter) Labels(ls Labels) {
	defer w.mu.Unlock()
	w.mu.Lock()

	b := w.buf[:0]

	sz := 0
	for _, l := range ls {
		q := len(l)
		sz += 1 + varintSize(uint64(q)) + q
	}

	b = appendVarint(b, uint64(sz))

	for _, l := range ls {
		b = append(b, 1<<3|2)
		b = appendVarint(b, uint64(len(l)))
		b = append(b, l...)
	}

	w.buf = b

	_, _ = w.w.Write(b)
}

// Message writes enent to the stream.
func (w *ProtoWriter) Message(m Message, s Span) {
	defer w.mu.Unlock()
	w.mu.Lock()

	if _, ok := w.ls[m.Location]; !ok {
		w.location(m.Location)
	}

	w.buf = w.buf[:0]

	l, _ := fmt.Fprintf(&w.buf, m.Format, m.Args...)

	sz := 0
	sz += 1 + varintSize(uint64(s.ID))
	sz += 1 + varintSize(uint64(m.Location))
	sz += 1 + varintSize(uint64(m.Time.Nanoseconds()/1000))
	sz += 1 + varintSize(uint64(l)) + l

	szs := varintSize(uint64(sz))
	szss := varintSize(uint64(1 + szs + sz))

	total := szss + 1 + szs + sz
	b := grow(w.buf, total)[:total]

	copy(b[total-l:], b[:l])

	b = appendVarint(b[:0], uint64(1+szs+sz))

	b = append(b, 3<<3|2)
	b = appendVarint(b, uint64(sz))

	b = append(b, 1<<3|0)
	b = appendVarint(b, uint64(s.ID))

	b = append(b, 2<<3|0)
	b = appendVarint(b, uint64(m.Location))

	b = append(b, 3<<3|0)
	b = appendVarint(b, uint64(m.Time.Nanoseconds()/1000))

	b = append(b, 4<<3|2)
	b = appendVarint(b, uint64(l))
	// text is already in place
	b = b[:total]

	w.buf = b

	_, _ = w.w.Write(b)
}

// SpanStarted writes event to the stream.
func (w *ProtoWriter) SpanStarted(s Span, par ID, loc Location) {
	defer w.mu.Unlock()
	w.mu.Lock()

	if _, ok := w.ls[loc]; !ok {
		w.location(loc)
	}

	sz := 0
	sz += 1 + varintSize(uint64(s.ID))
	if par != 0 {
		sz += 1 + varintSize(uint64(par))
	}
	sz += 1 + varintSize(uint64(loc))
	sz += 1 + varintSize(uint64(s.Started.UnixNano()/1000))

	b := w.buf[:0]
	szs := varintSize(uint64(sz))
	b = appendVarint(b, uint64(1+szs+sz))

	b = append(b, 4<<3|2)
	b = appendVarint(b, uint64(sz))

	b = append(b, 1<<3|0)
	b = appendVarint(b, uint64(s.ID))

	if par != 0 {
		b = append(b, 2<<3|0)
		b = appendVarint(b, uint64(par))
	}

	b = append(b, 3<<3|0)
	b = appendVarint(b, uint64(loc))

	b = append(b, 4<<3|0)
	b = appendVarint(b, uint64(s.Started.UnixNano()/1000))

	w.buf = b

	_, _ = w.w.Write(b)
}

// SpanFinished writes event to the stream.
func (w *ProtoWriter) SpanFinished(s Span, el time.Duration) {
	defer w.mu.Unlock()
	w.mu.Lock()

	sz := 0
	sz += 1 + varintSize(uint64(s.ID))
	sz += 1 + varintSize(uint64(el.Nanoseconds()/1000))

	b := w.buf[:0]
	szs := varintSize(uint64(sz))
	b = appendVarint(b, uint64(1+szs+sz))

	b = append(b, 5<<3|2)
	b = appendVarint(b, uint64(sz))

	b = append(b, 1<<3|0)
	b = appendVarint(b, uint64(s.ID))

	b = append(b, 2<<3|0)
	b = appendVarint(b, uint64(el.Nanoseconds()/1000))

	w.buf = b

	_, _ = w.w.Write(b)
}

func (w *ProtoWriter) location(l Location) {
	name, file, line := l.NameFileLine()

	b := w.buf[:0]

	sz := 0
	sz += 1 + varintSize(uint64(l))
	sz += 1 + varintSize(uint64(len(name))) + len(name)
	sz += 1 + varintSize(uint64(len(file))) + len(file)
	sz += 1 + varintSize(uint64(line))

	b = appendVarint(b, uint64(1+varintSize(uint64(sz))+sz))

	b = append(b, 2<<3|2)
	b = appendVarint(b, uint64(sz))

	b = append(b, 1<<3|0)
	b = appendVarint(b, uint64(l))

	b = append(b, 2<<3|2)
	b = appendVarint(b, uint64(len(name)))
	b = append(b, name...)

	b = append(b, 3<<3|2)
	b = appendVarint(b, uint64(len(file)))
	b = append(b, file...)

	b = append(b, 4<<3|0)
	b = appendVarint(b, uint64(line))

	w.ls[l] = struct{}{}
	w.buf = b

	_, _ = w.w.Write(b)
}

func appendVarint(b []byte, v uint64) []byte {
	switch {
	case v < 0x80:
		return append(b, byte(v))
	case v < 1<<14:
		return append(b, byte(v|0x80), byte(v>>7))
	case v < 1<<21:
		return append(b, byte(v|0x80), byte(v>>7|0x80), byte(v>>14))
	case v < 1<<28:
		return append(b, byte(v|0x80), byte(v>>7|0x80), byte(v>>14|0x80), byte(v>>21))
	case v < 1<<35:
		return append(b, byte(v|0x80), byte(v>>7|0x80), byte(v>>14|0x80), byte(v>>21|0x80), byte(v>>28))
	case v < 1<<42:
		return append(b, byte(v|0x80), byte(v>>7|0x80), byte(v>>14|0x80), byte(v>>21|0x80), byte(v>>28|0x80), byte(v>>35))
	case v < 1<<49:
		return append(b, byte(v|0x80), byte(v>>7|0x80), byte(v>>14|0x80), byte(v>>21|0x80), byte(v>>28|0x80), byte(v>>35|0x80),
			byte(v>>42))
	case v < 1<<56:
		return append(b, byte(v|0x80), byte(v>>7|0x80), byte(v>>14|0x80), byte(v>>21|0x80), byte(v>>28|0x80), byte(v>>35|0x80),
			byte(v>>42|0x80), byte(v>>49))
	case v < 1<<63:
		return append(b, byte(v|0x80), byte(v>>7|0x80), byte(v>>14|0x80), byte(v>>21|0x80), byte(v>>28|0x80), byte(v>>35|0x80),
			byte(v>>42|0x80), byte(v>>49|0x80), byte(v>>56))
	default:
		return append(b, byte(v|0x80), byte(v>>7|0x80), byte(v>>14|0x80), byte(v>>21|0x80), byte(v>>28|0x80), byte(v>>35|0x80),
			byte(v>>42|0x80), byte(v>>49|0x80), byte(v>>56), byte(v>>63))
	}
}

func varintSize(v uint64) int {
	s := 0
	for ; v != 0; v >>= 7 {
		s++
	}
	return s
}

// NewTeeWriter creates multiwriter that writes the same events to all writers in the same order.
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

func (w *TeeWriter) Message(m Message, s Span) {
	defer w.mu.Unlock()
	w.mu.Lock()

	for _, w := range w.Writers {
		w.Message(m, s)
	}
}

func (w *TeeWriter) SpanStarted(s Span, par ID, loc Location) {
	defer w.mu.Unlock()
	w.mu.Lock()

	for _, w := range w.Writers {
		w.SpanStarted(s, par, loc)
	}
}

func (w *TeeWriter) SpanFinished(s Span, el time.Duration) {
	defer w.mu.Unlock()
	w.mu.Lock()

	for _, w := range w.Writers {
		w.SpanFinished(s, el)
	}
}

func (w Discard) Labels(Labels)                    {}
func (w Discard) Message(Message, Span)            {}
func (w Discard) SpanStarted(Span, ID, Location)   {}
func (w Discard) SpanFinished(Span, time.Duration) {}

func (w *bufWriter) Write(p []byte) (int, error) {
	*w = append(*w, p...)
	return len(p), nil
}

func (w *bufWriter) NewLine() {
	l := len(*w)
	if l == 0 || (*w)[l-1] != '\n' {
		*w = append(*w, '\n')
	}
}
