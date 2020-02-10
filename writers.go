package tlog

import (
	"io"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:generate protoc --go_out=. tlogpb/tlog.proto

type (
	// ConsoleWriter produces similar output as stdlib log.Logger.
	//
	// Each event ends up with a single Write.
	//
	// It's unsafe to write event simultaneously.
	ConsoleWriter struct {
		w         io.Writer
		f         int
		Shortfile int
		Funcname  int
		IDWidth   int
		buf       bufWriter
	}

	// JSONWriter produces output readable by both machines and humans.
	//
	// Each event ends up with a single Write if message fits in 1000 bytes (default) buffer.
	//
	// It's unsafe to write event simultaneously.
	JSONWriter struct {
		w         io.Writer
		ls        map[Location]struct{}
		buf, buf2 []byte
	}

	// ProtobufWriter encodes event logs in protobuf and produces more compact output then JSONWriter.
	//
	// Each event ends up with a single Write.
	//
	// It's unsafe to write event simultaneously.
	ProtobufWriter struct {
		w   io.Writer
		ls  map[Location]struct{}
		buf bufWriter
	}

	// TeeWriter writes the same events in the same order to all Writers one after another.
	TeeWriter []Writer

	// Discard discards all events.
	Discard struct{}

	// LockedWriter is a Writer under Mutex
	// It's safe to write event simultaneously.
	LockedWriter struct {
		mu sync.Mutex
		w  Writer
	}

	bufWriter []byte
)

const TimeReduction = 6

// NewConsoleWriter creates writer with similar output as log.Logger.
func NewConsoleWriter(w io.Writer, f int) *ConsoleWriter {
	return &ConsoleWriter{
		w:         w,
		f:         f,
		Shortfile: 20,
		Funcname:  18,
		IDWidth:   16,
	}
}

func (w *ConsoleWriter) appendSegments(b []byte, W int, name string, s byte) []byte {
	end := len(b) + W
	for len(b) < end {
		if len(name) <= end-len(b) {
			b = append(b, name...)
			break
		}

		p := strings.IndexByte(name, s)
		if p == -1 {
			b = append(b, name[:end-len(b)]...)
			break
		}

		b = append(b, name[0], s)

		name = name[p+1:]
	}

	return b
}

func (w *ConsoleWriter) buildHeader(loc Location, t time.Time) {
	b := w.buf[:0]

	var fname, file string
	var line = -1

	if w.f&(Ldate|Ltime|Lmilliseconds|Lmicroseconds) != 0 {
		if w.f&LUTC != 0 {
			t = t.UTC()
		}

		var Y, M, D, h, m, s int
		if w.f&(Ldate|Ltime) != 0 {
			Y, M, D, h, m, s = splitTime(t)
		}

		if w.f&Ldate != 0 {
			i := len(b)
			b = append(b, "0000/00/00"...)

			for j := 0; j < 4; j++ {
				b[i+3-j] = byte(Y%10) + '0'
				Y /= 10
			}

			b[i+6] = byte(M%10) + '0'
			M /= 10
			b[i+5] = byte(M) + '0'

			b[i+9] = byte(D%10) + '0'
			D /= 10
			b[i+8] = byte(D) + '0'
		}
		if w.f&Ltime != 0 {
			if len(b) != 0 {
				b = append(b, '_')
			}

			i := len(b)
			b = append(b, "00:00:00"...)

			b[i+1] = byte(h%10) + '0'
			h /= 10
			b[i+0] = byte(h) + '0'

			b[i+4] = byte(m%10) + '0'
			m /= 10
			b[i+3] = byte(m) + '0'

			b[i+7] = byte(s%10) + '0'
			s /= 10
			b[i+6] = byte(s) + '0'
		}
		if w.f&(Lmilliseconds|Lmicroseconds) != 0 {
			if len(b) != 0 {
				b = append(b, '.')
			}

			ns := t.Nanosecond() / 1e3
			if w.f&Lmilliseconds != 0 {
				ns /= 1000

				i := len(b)
				b = append(b, "000"...)

				b[i+2] = byte(ns%10) + '0'
				ns /= 10
				b[i+1] = byte(ns%10) + '0'
				ns /= 10
				b[i+0] = byte(ns%10) + '0'
			} else {
				i := len(b)
				b = append(b, "000000"...)

				b[i+5] = byte(ns%10) + '0'
				ns /= 10
				b[i+4] = byte(ns%10) + '0'
				ns /= 10
				b[i+3] = byte(ns%10) + '0'
				ns /= 10
				b[i+2] = byte(ns%10) + '0'
				ns /= 10
				b[i+1] = byte(ns%10) + '0'
				ns /= 10
				b[i+0] = byte(ns%10) + '0'
			}
		}

		b = append(b, ' ', ' ')
	}
	if w.f&(Llongfile|Lshortfile) != 0 {
		fname, file, line = loc.NameFileLine()

		if w.f&Lshortfile != 0 {
			file = filepath.Base(file)

			n := 1
			for q := line; q != 0; q /= 10 {
				n++
			}

			i := len(b)

			b = append(b, "                                                                                                                                                "[:w.Shortfile]...)
			b = append(b[:i], file...)

			e := len(b)
			b = b[:i+w.Shortfile]

			if len(file)+n > w.Shortfile {
				i = i + w.Shortfile - n
			} else {
				i = e
			}

			b[i] = ':'
			for q, j := line, n-1; j >= 1; j-- {
				b[i+j] = byte(q%10) + '0'
				q /= 10
			}
		} else {
			b = append(b, file...)
			i := len(b)
			b = append(b, ":           "...)

			n := 1
			for q := line; q != 0; q /= 10 {
				n++
			}

			for q, j := line, n-1; j >= 1; j-- {
				b[i+j] = byte(q%10) + '0'
				q /= 10
			}
		}

		b = append(b, ' ', ' ')
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

			if l := len(fname); l <= w.Funcname {
				i := len(b)
				b = append(b, "                                                                                                                                                "[:w.Funcname]...)
				b = append(b[:i], fname...)
				b = b[:i+w.Funcname]
			} else {
				b = append(b, fname[:w.Funcname]...)
				j := 1
				for {
					q := fname[l-j]
					if q < '0' || '9' < q {
						break
					}
					b[len(b)-j] = q
					j++
				}
			}
		} else {
			b = append(b, fname...)
		}

		b = append(b, ' ', ' ')
	}

	w.buf = b
}

// Message writes Message event by single Write.
func (w *ConsoleWriter) Message(m Message, s Span) {
	var t time.Time
	if s.ID != z {
		t = s.Started.Add(m.Time)
	} else {
		t = m.AbsTime()
	}

	w.buildHeader(m.Location, t)

	if w.f&Lmessagespan != 0 {
		i := len(w.buf)
		b := append(w.buf, "123456789_123456789_123456789_12"[:w.IDWidth]...)
		s.ID.FormatTo(b[i:i+w.IDWidth], 'v')

		w.buf = append(b, ' ', ' ')
	}

	if m.Args != nil {
		w.buf = AppendPrintf(w.buf, m.Format, m.Args...)
	} else {
		w.buf = append(w.buf, m.Format...)
	}

	w.buf.NewLine()

	_, _ = w.w.Write(w.buf)
}

func (w *ConsoleWriter) spanHeader(sid, par ID, loc Location, tm time.Time) {
	w.buildHeader(loc, tm)

	i := len(w.buf)
	b := append(w.buf, "123456789_123456789_123456789_12"[:w.IDWidth]...)
	sid.FormatTo(b[i:i+w.IDWidth], 'v')

	w.buf = append(b, ' ', ' ')
}

// Message writes SpanStarted event by single Write.
func (w *ConsoleWriter) SpanStarted(s Span, par ID, l Location) {
	if w.f&Lspans == 0 {
		return
	}

	w.spanHeader(s.ID, par, l, s.Started)

	if par == z {
		w.buf = append(w.buf, "Span started\n"...)
	} else {
		b := append(w.buf, "Span spawned from "...)

		i := len(b)
		b = append(b, "123456789_123456789_123456789_12"[:w.IDWidth]...)
		par.FormatTo(b[i:i+w.IDWidth], 'v')

		w.buf = append(b, '\n')
	}

	_, _ = w.w.Write(w.buf)
}

// Message writes SpanFinished event by single Write.
func (w *ConsoleWriter) SpanFinished(s Span, el time.Duration) {
	if w.f&Lspans == 0 {
		return
	}

	w.spanHeader(s.ID, z, 0, s.Started.Add(el))

	b := append(w.buf, "Span finished - elapsed "...)

	e := el.Seconds() * 1000
	b = strconv.AppendFloat(b, e, 'f', 2, 64)

	w.buf = append(b, "ms\n"...)

	_, _ = w.w.Write(w.buf)
}

// Message writes Labels by single Write.
func (w *ConsoleWriter) Labels(ls Labels) {
	var buf [4]Location
	StackTraceFill(1, buf[:])
	i := 0
	for i+1 < len(buf) {
		name, _, _ := buf[i].NameFileLine()
		name = path.Base(name)
		if strings.HasPrefix(name, "tlog.") {
			i++
			continue
		}
		break
	}

	w.Message(
		Message{
			Location: buf[i],
			Time:     time.Duration(now().UnixNano()),
			Format:   "Labels: %q",
			Args:     []interface{}{ls},
		},
		Span{},
	)
}

// NewConsoleWriter creates JSON writer.
func NewJSONWriter(w io.Writer) *JSONWriter {
	return &JSONWriter{
		w:  w,
		ls: make(map[Location]struct{}),
	}
}

// Labels writes Labels to the stream.
func (w *JSONWriter) Labels(ls Labels) {
	b := w.buf

	b = append(b, `{"L":[`...)

	for i, l := range ls {
		if i == 0 {
			b = append(b, '"')
		} else {
			b = append(b, ',', '"')
		}
		b = appendSafe(b, l)
		b = append(b, '"')
	}

	b = append(b, "]}\n"...)

	w.buf = b[:0]

	_, _ = w.w.Write(b)
}

// Message writes event to the stream.
func (w *JSONWriter) Message(m Message, s Span) {
	if _, ok := w.ls[m.Location]; !ok {
		w.location(m.Location)
	}

	b := w.buf

	b = append(b, `{"m":{`...)

	if s.ID != z {
		b = append(b, `"s":"`...)
		i := len(b)
		b = append(b, `123456789_123456789_123456789_12",`...)
		s.ID.FormatTo(b[i:], 'x')
	}

	b = append(b, `"t":`...)
	b = strconv.AppendInt(b, m.Time.Nanoseconds()>>TimeReduction, 10)

	if m.Location != 0 {
		b = append(b, `,"l":`...)
		b = strconv.AppendInt(b, int64(m.Location), 10)
	}

	b = append(b, `,"m":"`...)
	if m.Args != nil {
		w.buf2 = AppendPrintf(w.buf2[:0], m.Format, m.Args...)
		b = append(b, w.buf2...)
	} else {
		cv := stringToBytes(m.Format)
		b = append(b, cv...)
	}

	b = append(b, "\"}}\n"...)

	w.buf = b[:0]

	_, _ = w.w.Write(b)
}

// SpanStarted writes event to the stream.
func (w *JSONWriter) SpanStarted(s Span, par ID, loc Location) {
	if _, ok := w.ls[loc]; !ok {
		w.location(loc)
	}

	b := w.buf

	b = append(b, `{"s":{"i":"`...)
	i := len(b)
	b = append(b, `123456789_123456789_123456789_12"`...)
	s.ID.FormatTo(b[i:], 'x')

	b = append(b, `,"s":`...)
	b = strconv.AppendInt(b, s.Started.UnixNano()>>TimeReduction, 10)

	b = append(b, `,"l":`...)
	b = strconv.AppendInt(b, int64(loc), 10)

	if par != z {
		b = append(b, `,"p":"`...)
		i = len(b)
		b = append(b, `123456789_123456789_123456789_12"`...)
		par.FormatTo(b[i:], 'x')
	}

	b = append(b, "}}\n"...)

	w.buf = b[:0]

	_, _ = w.w.Write(b)
}

// SpanFinished writes event to the stream.
func (w *JSONWriter) SpanFinished(s Span, el time.Duration) {
	b := w.buf

	b = append(b, `{"f":{"i":"`...)
	i := len(b)
	b = append(b, `123456789_123456789_123456789_12"`...)
	s.ID.FormatTo(b[i:], 'x')

	b = append(b, `,"e":`...)
	b = strconv.AppendInt(b, el.Nanoseconds()>>TimeReduction, 10)

	b = append(b, "}}\n"...)

	w.buf = b[:0]

	_, _ = w.w.Write(b)
}

func (w *JSONWriter) location(l Location) {
	if l == 0 {
		return
	}

	name, file, line := l.NameFileLine()
	//	name = path.Base(name)

	b := w.buf

	b = append(b, `{"l":{"p":`...)
	b = strconv.AppendInt(b, int64(l), 10)

	b = append(b, `,"f":"`...)
	b = appendSafe(b, file)

	b = append(b, `","l":`...)
	b = strconv.AppendInt(b, int64(line), 10)

	b = append(b, `,"n":"`...)
	b = appendSafe(b, name)

	b = append(b, "\"}}\n"...)

	w.ls[l] = struct{}{}
	w.buf = b
}

// NewConsoleWriter creates Protobuf writer.
func NewProtobufWriter(w io.Writer) *ProtobufWriter {
	return &ProtobufWriter{
		w:  w,
		ls: make(map[Location]struct{}),
	}
}

// Labels writes Labels to the stream.
func (w *ProtobufWriter) Labels(ls Labels) {
	sz := 0
	for _, l := range ls {
		q := len(l)
		sz += 1 + varintSize(uint64(q)) + q
	}

	szs := varintSize(uint64(sz))

	b := w.buf[:0]
	b = appendVarint(b, uint64(1+szs+sz))

	b = appendTagVarint(b, 1<<3|2, uint64(sz))

	for _, l := range ls {
		b = appendTagVarint(b, 1<<3|2, uint64(len(l)))
		b = append(b, l...)
	}

	w.buf = b

	_, _ = w.w.Write(b)
}

// Message writes enent to the stream.
func (w *ProtobufWriter) Message(m Message, s Span) {
	if _, ok := w.ls[m.Location]; !ok {
		w.location(m.Location)
	}

	var b []byte
	var l int
	if m.Args != nil {
		b = AppendPrintf(w.buf[:0], m.Format, m.Args...)
	} else {
		b = append(w.buf[:0], m.Format...)
	}
	l = len(b)

	sz := 0
	if s.ID != z {
		sz += 1 + varintSize(uint64(len(s.ID))) + len(s.ID)
	}
	if m.Location != 0 {
		sz += 1 + varintSize(uint64(m.Location))
	}
	sz += 1 + varintSize(uint64(m.Time.Nanoseconds()>>TimeReduction))
	sz += 1 + varintSize(uint64(l)) + l

	szs := varintSize(uint64(sz))
	szss := varintSize(uint64(1 + szs + sz))

	total := szss + 1 + szs + sz
	for cap(b) < total {
		b = append(b, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	}
	b = b[:total]

	copy(b[total-l:], b[:l])

	b = appendVarint(b[:0], uint64(1+szs+sz))

	b = appendTagVarint(b, 3<<3|2, uint64(sz))

	if s.ID != z {
		b = appendTagVarint(b, 1<<3|2, uint64(len(s.ID)))
		b = append(b, s.ID[:]...)
	}

	if m.Location != 0 {
		b = appendTagVarint(b, 2<<3|0, uint64(m.Location))
	}

	b = appendTagVarint(b, 3<<3|0, uint64(m.Time.Nanoseconds()>>TimeReduction))

	b = appendTagVarint(b, 4<<3|2, uint64(l))
	// text is already in place
	b = b[:total]

	w.buf = b

	_, _ = w.w.Write(b)
}

// SpanStarted writes event to the stream.
func (w *ProtobufWriter) SpanStarted(s Span, par ID, loc Location) {
	if _, ok := w.ls[loc]; !ok {
		w.location(loc)
	}

	sz := 0
	sz += 1 + varintSize(uint64(len(s.ID))) + len(s.ID)
	if par != z {
		sz += 1 + varintSize(uint64(len(par))) + len(par)
	}
	if loc != 0 {
		sz += 1 + varintSize(uint64(loc))
	}
	sz += 1 + varintSize(uint64(s.Started.UnixNano()>>TimeReduction))

	b := w.buf[:0]
	szs := varintSize(uint64(sz))
	b = appendVarint(b, uint64(1+szs+sz))

	b = appendTagVarint(b, 4<<3|2, uint64(sz))

	b = appendTagVarint(b, 1<<3|2, uint64(len(s.ID)))
	b = append(b, s.ID[:]...)

	if par != z {
		b = appendTagVarint(b, 2<<3|2, uint64(len(par)))
		b = append(b, par[:]...)
	}

	if loc != 0 {
		b = appendTagVarint(b, 3<<3|0, uint64(loc))
	}

	b = appendTagVarint(b, 4<<3|0, uint64(s.Started.UnixNano()>>TimeReduction))

	w.buf = b

	_, _ = w.w.Write(b)
}

// SpanFinished writes event to the stream.
func (w *ProtobufWriter) SpanFinished(s Span, el time.Duration) {
	sz := 0
	sz += 1 + varintSize(uint64(len(s.ID))) + len(s.ID)
	sz += 1 + varintSize(uint64(el.Nanoseconds()>>TimeReduction))

	b := w.buf[:0]
	szs := varintSize(uint64(sz))
	b = appendVarint(b, uint64(1+szs+sz))

	b = appendTagVarint(b, 5<<3|2, uint64(sz))

	b = appendTagVarint(b, 1<<3|2, uint64(len(s.ID)))
	b = append(b, s.ID[:]...)

	b = appendTagVarint(b, 2<<3|0, uint64(el.Nanoseconds()>>TimeReduction))

	w.buf = b

	_, _ = w.w.Write(b)
}

func (w *ProtobufWriter) location(l Location) {
	if l == 0 {
		return
	}

	name, file, line := l.NameFileLine()

	b := w.buf[:0]

	sz := 0
	sz += 1 + varintSize(uint64(l))
	sz += 1 + varintSize(uint64(len(name))) + len(name)
	sz += 1 + varintSize(uint64(len(file))) + len(file)
	sz += 1 + varintSize(uint64(line))

	b = appendVarint(b, uint64(1+varintSize(uint64(sz))+sz))

	b = appendTagVarint(b, 2<<3|2, uint64(sz))

	b = appendTagVarint(b, 1<<3|0, uint64(l))

	b = appendTagVarint(b, 2<<3|2, uint64(len(name)))
	b = append(b, name...)

	b = appendTagVarint(b, 3<<3|2, uint64(len(file)))
	b = append(b, file...)

	b = appendTagVarint(b, 4<<3|0, uint64(line))

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

func appendTagVarint(b []byte, t byte, v uint64) []byte {
	switch {
	case v < 0x80:
		return append(b, t, byte(v))
	case v < 1<<14:
		return append(b, t, byte(v|0x80), byte(v>>7))
	case v < 1<<21:
		return append(b, t, byte(v|0x80), byte(v>>7|0x80), byte(v>>14))
	case v < 1<<28:
		return append(b, t, byte(v|0x80), byte(v>>7|0x80), byte(v>>14|0x80), byte(v>>21))
	case v < 1<<35:
		return append(b, t, byte(v|0x80), byte(v>>7|0x80), byte(v>>14|0x80), byte(v>>21|0x80), byte(v>>28))
	case v < 1<<42:
		return append(b, t, byte(v|0x80), byte(v>>7|0x80), byte(v>>14|0x80), byte(v>>21|0x80), byte(v>>28|0x80), byte(v>>35))
	case v < 1<<49:
		return append(b, t, byte(v|0x80), byte(v>>7|0x80), byte(v>>14|0x80), byte(v>>21|0x80), byte(v>>28|0x80), byte(v>>35|0x80),
			byte(v>>42))
	case v < 1<<56:
		return append(b, t, byte(v|0x80), byte(v>>7|0x80), byte(v>>14|0x80), byte(v>>21|0x80), byte(v>>28|0x80), byte(v>>35|0x80),
			byte(v>>42|0x80), byte(v>>49))
	case v < 1<<63:
		return append(b, t, byte(v|0x80), byte(v>>7|0x80), byte(v>>14|0x80), byte(v>>21|0x80), byte(v>>28|0x80), byte(v>>35|0x80),
			byte(v>>42|0x80), byte(v>>49|0x80), byte(v>>56))
	default:
		return append(b, t, byte(v|0x80), byte(v>>7|0x80), byte(v>>14|0x80), byte(v>>21|0x80), byte(v>>28|0x80), byte(v>>35|0x80),
			byte(v>>42|0x80), byte(v>>49|0x80), byte(v>>56), byte(v>>63))
	}
}

func varintSize(v uint64) int {
	switch {
	case v < 0x80:
		return 1
	case v < 1<<14:
		return 2
	case v < 1<<21:
		return 3
	case v < 1<<28:
		return 4
	case v < 1<<35:
		return 5
	case v < 1<<42:
		return 6
	case v < 1<<49:
		return 7
	case v < 1<<56:
		return 8
	case v < 1<<63:
		return 9
	default:
		return 10
	}
}

// NewTeeWriter creates multiwriter that writes the same events to all writers in the same order.
func NewTeeWriter(w ...Writer) TeeWriter {
	var ws []Writer
	for _, w := range w {
		if t, ok := w.(TeeWriter); ok {
			ws = append(ws, t...)
		} else {
			ws = append(ws, w)
		}
	}
	return TeeWriter(ws)
}

func (w TeeWriter) Labels(ls Labels) {
	for _, w := range w {
		w.Labels(ls)
	}
}

func (w TeeWriter) Message(m Message, s Span) {
	for _, w := range w {
		w.Message(m, s)
	}
}

func (w TeeWriter) SpanStarted(s Span, par ID, loc Location) {
	for _, w := range w {
		w.SpanStarted(s, par, loc)
	}
}

func (w TeeWriter) SpanFinished(s Span, el time.Duration) {
	for _, w := range w {
		w.SpanFinished(s, el)
	}
}

func (w Discard) Labels(Labels)                    {}
func (w Discard) Message(Message, Span)            {}
func (w Discard) SpanStarted(Span, ID, Location)   {}
func (w Discard) SpanFinished(Span, time.Duration) {}

func NewLockedWriter(w Writer) *LockedWriter {
	return &LockedWriter{w: w}
}

func (w *LockedWriter) Labels(ls Labels) {
	w.mu.Lock()
	w.w.Labels(ls)
	w.mu.Unlock()
}

func (w *LockedWriter) Message(m Message, s Span) {
	w.mu.Lock()
	w.w.Message(m, s)
	w.mu.Unlock()
}

func (w *LockedWriter) SpanStarted(s Span, par ID, loc Location) {
	w.mu.Lock()
	w.w.SpanStarted(s, par, loc)
	w.mu.Unlock()
}

func (w *LockedWriter) SpanFinished(s Span, el time.Duration) {
	w.mu.Lock()
	w.w.SpanFinished(s, el)
	w.mu.Unlock()
}

/*
func (w *bufWriter) Write(p []byte) (int, error) {
	*w = append(*w, p...)
	return len(p), nil
}
*/

func (w *bufWriter) NewLine() {
	l := len(*w)
	if l == 0 || (*w)[l-1] != '\n' {
		*w = append(*w, '\n')
	}
}
