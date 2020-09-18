package tlog

import (
	"encoding/binary"
	"io"
	"math"
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
	}

	// JSONWriter produces output readable by both machines and humans.
	//
	// Each event ends up with a single Write if message fits in 1000 bytes (default) buffer.
	//
	// It's unsafe to write event simultaneously.
	JSONWriter struct {
		w io.Writer
		writerCache
	}

	// ProtoWriter encodes event logs in protobuf and produces more compact output then JSONWriter.
	//
	// Each event ends up with a single Write.
	//
	// It's unsafe to write event simultaneously.
	ProtoWriter struct {
		w io.Writer
		writerCache
	}

	writerCache struct {
		mu   sync.Mutex //nolint:structcheck
		ls   map[Location]struct{}
		cc   map[uintptr][]mh
		skip map[string]int
		ccn  int
	}

	mh struct {
		N      int
		Name   string
		Labels Labels
	}

	// TeeWriter writes the same events in the same order to all Writers one after another.
	TeeWriter []Writer

	// DiscardWriter discards all events.
	DiscardWriter struct{}

	// LockedWriter is a Writer under Mutex
	// It's safe to write event simultaneously.
	LockedWriter struct {
		mu sync.Mutex
		w  Writer
	}

	// FallbackWriter writes all the events to Writer.
	// If non-nil error was returned the same event is passed to Fallback.
	// Writer error is returned anyway.
	FallbackWriter struct {
		Writer   Writer
		Fallback Writer
	}

	bufWriter []byte

	bwr struct {
		b bufWriter
	}

	LockedIOWriter struct {
		mu sync.Mutex
		w  io.Writer
	}
)

const metricsSkipAt = 1000

var ( // type checks
	_ Writer = &ConsoleWriter{}
	_ Writer = &JSONWriter{}
	_ Writer = &ProtoWriter{}
	_ Writer = &TeeWriter{}
	_ Writer = &LockedWriter{}
	_ Writer = FallbackWriter{}

	Discard Writer = DiscardWriter{}
)

var spaces = []byte("                                                                                                                                                ")

var bufPool = sync.Pool{New: func() interface{} { return &bwr{b: make(bufWriter, 1000)} }}

func getbuf() (bufWriter, *bwr) {
	w := bufPool.Get().(*bwr)
	return w.b[:0], w
}

func retbuf(b bufWriter, w *bwr) {
	w.b = b
	bufPool.Put(w)
}

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

func (w *ConsoleWriter) appendSegments(b []byte, wid int, name string, s byte) []byte {
	end := len(b) + wid
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

//nolint:gocognit,nestif
func (w *ConsoleWriter) buildHeader(b []byte, ts int64, loc Location) []byte {
	var fname, file string
	line := -1

	if w.f&(Ldate|Ltime|Lmilliseconds|Lmicroseconds) != 0 {
		t := time.Unix(0, ts)

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

			b = append(b, spaces[:w.Shortfile]...)
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
				b = append(b, spaces[:w.Funcname]...)
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

	return b
}

// Message writes Message event by single Write.
func (w *ConsoleWriter) Message(m Message, sid ID) (err error) {
	b, wr := getbuf()
	defer retbuf(b, wr)

	b = w.buildHeader(b, m.Time, m.Location)

	if w.f&Lmessagespan != 0 {
		i := len(b)
		b = append(b, "123456789_123456789_123456789_12"[:w.IDWidth]...)
		sid.FormatTo(b[i:i+w.IDWidth], 'v')

		b = append(b, ' ', ' ')
	}

	switch {
	case m.Args == nil:
		b = append(b, m.Format...)
	case m.Format == "":
		b = AppendPrintln(b, m.Args...)
	default:
		b = AppendPrintf(b, m.Format, m.Args...)
	}

	b.NewLine()

	_, err = w.w.Write(b)

	return
}

func (w *ConsoleWriter) spanHeader(b []byte, sid, par ID, tm int64, loc Location) []byte {
	b = w.buildHeader(b, tm, loc)

	i := len(b)
	b = append(b, "123456789_123456789_123456789_12"[:w.IDWidth]...)
	sid.FormatTo(b[i:i+w.IDWidth], 'v')

	return append(b, ' ', ' ')
}

// SpanStarted writes SpanStarted event by single Write.
func (w *ConsoleWriter) SpanStarted(s SpanStart) (err error) {
	if w.f&Lspans == 0 {
		return
	}

	b, wr := getbuf()
	defer retbuf(b, wr)

	b = w.spanHeader(b, s.ID, s.Parent, s.Started, s.Location)

	if s.Parent == (ID{}) {
		b = append(b, "Span started\n"...)
	} else {
		b = append(b, "Span spawned from "...)

		i := len(b)
		b = append(b, "123456789_123456789_123456789_12"[:w.IDWidth]...)
		s.Parent.FormatTo(b[i:i+w.IDWidth], 'v')

		b = append(b, '\n')
	}

	_, err = w.w.Write(b)

	return
}

// SpanFinished writes SpanFinished event by single Write.
func (w *ConsoleWriter) SpanFinished(f SpanFinish) (err error) {
	if w.f&Lspans == 0 {
		return
	}

	b, wr := getbuf()
	defer retbuf(b, wr)

	b = w.spanHeader(b, f.ID, ID{}, now(), 0)

	b = append(b, "Span finished - elapsed "...)

	e := time.Duration(f.Elapsed).Seconds() * 1000
	b = strconv.AppendFloat(b, e, 'f', 2, 64)

	b = append(b, "ms\n"...)

	_, err = w.w.Write(b)

	return
}

// Labels writes Labels by single Write.
func (w *ConsoleWriter) Labels(ls Labels, sid ID) error {
	loc := w.caller()

	return w.Message(
		Message{
			Location: loc,
			Time:     now(),
			Format:   "Labels: %q",
			Args:     []interface{}{ls},
		},
		sid,
	)
}

func (w *ConsoleWriter) Meta(m Meta) error {
	loc := w.caller()

	return w.Message(
		Message{
			Location: loc,
			Time:     now(),
			Format:   "Meta: %v %q",
			Args:     []interface{}{m.Type, m.Data},
		},
		ID{},
	)
}

func (w *ConsoleWriter) Metric(m Metric, sid ID) error {
	loc := w.caller()

	return w.Message(
		Message{
			Location: loc,
			Time:     now(),
			Format:   "%v  %15.5f  %v",
			Args:     []interface{}{m.Name, m.Value, m.Labels},
		},
		sid,
	)
}

func (w *ConsoleWriter) caller() Location {
	var buf [6]Location
	FillStackTrace(2, buf[:])
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

	return buf[i]
}

func makeWriteCache() writerCache {
	return writerCache{
		ls:   make(map[Location]struct{}),
		cc:   make(map[uintptr][]mh),
		skip: make(map[string]int),
	}
}

// NewConsoleWriter creates JSON writer.
func NewJSONWriter(w io.Writer) *JSONWriter {
	return &JSONWriter{
		w:           w,
		writerCache: makeWriteCache(),
	}
}

// Labels writes Labels to the stream.
func (w *JSONWriter) Labels(ls Labels, sid ID) (err error) {
	b, wr := getbuf()
	defer retbuf(b, wr)

	b = append(b, `{"L":{`...)

	if sid != (ID{}) {
		b = append(b, `"s":"`...)
		i := len(b)
		b = append(b, `123456789_123456789_123456789_12",`...)
		sid.FormatTo(b[i:], 'x')
	}

	b = append(b, `"L":[`...)

	for i, l := range ls {
		if i == 0 {
			b = append(b, '"')
		} else {
			b = append(b, ',', '"')
		}
		b = appendSafe(b, l)
		b = append(b, '"')
	}

	b = append(b, "]}}\n"...)

	_, err = w.w.Write(b)

	return
}

func (w *JSONWriter) Meta(m Meta) (err error) {
	b, wr := getbuf()
	defer retbuf(b, wr)

	b = append(b, `{"M":{"t":"`...)
	b = appendSafe(b, m.Type)
	b = append(b, '"')

	if len(m.Data) != 0 {
		b = append(b, `,"d":[`...)

		for i, l := range m.Data {
			if i != 0 {
				b = append(b, ',')
			}

			b = append(b, '"')
			b = appendSafe(b, l)
			b = append(b, '"')
		}

		b = append(b, ']')
	}

	b = append(b, "}}\n"...)

	_, err = w.w.Write(b)

	return
}

// Message writes event to the stream.
func (w *JSONWriter) Message(m Message, sid ID) (err error) {
	b, wr := getbuf()
	defer retbuf(b, wr)

	if m.Location != 0 {
		w.mu.Lock()

		if _, ok := w.ls[m.Location]; !ok {
			defer w.mu.Unlock()

			b = w.location(b, m.Location)
		} else {
			w.mu.Unlock()
		}
	}

	b = append(b, `{"m":{`...)

	if sid != (ID{}) {
		b = append(b, `"s":"`...)
		i := len(b)
		b = append(b, `123456789_123456789_123456789_12",`...)
		sid.FormatTo(b[i:], 'x')
	}

	b = append(b, `"t":`...)
	b = strconv.AppendInt(b, m.Time, 10)

	if m.Location != 0 {
		b = append(b, `,"l":`...)
		b = strconv.AppendInt(b, int64(m.Location), 10)
	}

	b = append(b, `,"m":"`...)
	switch {
	case m.Args == nil:
		b = append(b, m.Format...)
	case m.Format == "":
		b = AppendPrintln(b, m.Args...)
	default:
		b = AppendPrintf(b, m.Format, m.Args...)
	}

	b = append(b, "\"}}\n"...)

	_, err = w.w.Write(b)

	return
}

func (w *writerCache) killMetric(n string) {
	w.skip[n] = -1

	for h, list := range w.cc {
		eq := 0
		for _, el := range list {
			if el.Name == n {
				eq++
			}
		}

		if eq == 0 {
			continue
		}

		if eq == len(list) {
			delete(w.cc, h)

			continue
		}

		var cp []mh
		for _, el := range list {
			if el.Name == n {
				continue
			}

			cp = append(cp, el)
		}

		w.cc[h] = cp
	}
}

func (w *writerCache) metricCached(m Metric) (cnum int, had bool) {
	skip := w.skip[m.Name]
	if skip == -1 {
		return 0, false
	}

	if skip > metricsSkipAt {
		w.killMetric(m.Name)

		return 0, false
	}

	n := m.Name
	h := strhash(&n, 0)
	for _, l := range m.Labels {
		n = l
		h = strhash(&n, h)
	}

	cnum = -1
	had = true
outer:
	for _, c := range w.cc[h] {
		if len(c.Labels) != len(m.Labels) {
			continue
		}

		if c.Name != m.Name {
			continue
		}

		for i, l := range c.Labels {
			if l != m.Labels[i] {
				continue outer
			}
		}

		cnum = c.N
	}

	if cnum == -1 {
		w.ccn++
		cnum = w.ccn
		had = false

		w.skip[m.Name]++

		ls := make(Labels, len(m.Labels))
		copy(ls, m.Labels)

		w.cc[h] = append(w.cc[h], mh{
			N:      cnum,
			Name:   m.Name,
			Labels: ls,
		})
	}

	return
}

func (w *JSONWriter) Metric(m Metric, sid ID) (err error) {
	w.mu.Lock()
	cnum, had := w.metricCached(m)
	w.mu.Unlock()

	b, wr := getbuf()
	defer retbuf(b, wr)

	b = append(b, `{"v":{`...)

	if sid != (ID{}) {
		b = append(b, `"s":"`...)
		i := len(b)
		b = append(b, `123456789_123456789_123456789_12",`...)
		sid.FormatTo(b[i:], 'x')
	}

	b = append(b, `"h":`...)
	b = strconv.AppendInt(b, int64(cnum), 10)

	b = append(b, `,"v":`...)
	b = strconv.AppendFloat(b, m.Value, 'g', -1, 64)

	if !had {
		b = append(b, `,"n":"`...)
		b = appendSafe(b, m.Name)
		b = append(b, '"')

		if len(m.Labels) != 0 {
			b = append(b, `,"L":[`...)
			for i, l := range m.Labels {
				if i != 0 {
					b = append(b, ',')
				}
				b = append(b, '"')
				b = appendSafe(b, l)
				b = append(b, '"')
			}
			b = append(b, ']')
		}
	}

	b = append(b, `}}`+"\n"...)

	_, err = w.w.Write(b)

	return
}

// SpanStarted writes event to the stream.
func (w *JSONWriter) SpanStarted(s SpanStart) (err error) {
	b, wr := getbuf()
	defer retbuf(b, wr)

	if s.Location != 0 {
		w.mu.Lock()

		if _, ok := w.ls[s.Location]; !ok {
			defer w.mu.Unlock()

			b = w.location(b, s.Location)
		} else {
			w.mu.Unlock()
		}
	}

	b = append(b, `{"s":{"i":"`...)
	i := len(b)
	b = append(b, `123456789_123456789_123456789_12"`...)
	s.ID.FormatTo(b[i:], 'x')

	b = append(b, `,"s":`...)
	b = strconv.AppendInt(b, s.Started, 10)

	b = append(b, `,"l":`...)
	b = strconv.AppendInt(b, int64(s.Location), 10)

	if s.Parent != (ID{}) {
		b = append(b, `,"p":"`...)
		i = len(b)
		b = append(b, `123456789_123456789_123456789_12"`...)
		s.Parent.FormatTo(b[i:], 'x')
	}

	b = append(b, "}}\n"...)

	_, err = w.w.Write(b)

	return
}

// SpanFinished writes event to the stream.
func (w *JSONWriter) SpanFinished(f SpanFinish) (err error) {
	b, wr := getbuf()
	defer retbuf(b, wr)

	b = append(b, `{"f":{"i":"`...)
	i := len(b)
	b = append(b, `123456789_123456789_123456789_12"`...)
	f.ID.FormatTo(b[i:], 'x')

	b = append(b, `,"e":`...)
	b = strconv.AppendInt(b, f.Elapsed, 10)

	b = append(b, "}}\n"...)

	_, err = w.w.Write(b)

	return
}

func (w *JSONWriter) location(b []byte, l Location) []byte {
	name, file, line := l.NameFileLine()
	//	name = path.Base(name)

	b = append(b, `{"l":{"p":`...)
	b = strconv.AppendInt(b, int64(l), 10)

	b = append(b, `,"e":`...)
	b = strconv.AppendInt(b, int64(l.Entry()), 10)

	b = append(b, `,"f":"`...)
	b = appendSafe(b, file)

	b = append(b, `","l":`...)
	b = strconv.AppendInt(b, int64(line), 10)

	b = append(b, `,"n":"`...)
	b = appendSafe(b, name)

	b = append(b, "\"}}\n"...)

	w.ls[l] = struct{}{}

	return b
}

// NewConsoleWriter creates protobuf writer.
func NewProtoWriter(w io.Writer) *ProtoWriter {
	return &ProtoWriter{
		w:           w,
		writerCache: makeWriteCache(),
	}
}

// Labels writes Labels to the stream.
func (w *ProtoWriter) Labels(ls Labels, sid ID) (err error) {
	sz := 0

	if sid != (ID{}) {
		sz += 1 + varintSize(uint64(len(sid))) + len(sid)
	}

	for _, l := range ls {
		q := len(l)
		sz += 1 + varintSize(uint64(q)) + q
	}

	szs := varintSize(uint64(sz))

	b, wr := getbuf()
	defer retbuf(b, wr)

	b = appendVarint(b, uint64(1+szs+sz))

	b = appendTagVarint(b, 1<<3|2, uint64(sz))

	if sid != (ID{}) {
		b = appendTagVarint(b, 1<<3|2, uint64(len(sid)))
		b = append(b, sid[:]...)
	}

	for _, l := range ls {
		b = appendTagVarint(b, 2<<3|2, uint64(len(l)))
		b = append(b, l...)
	}

	_, err = w.w.Write(b)

	return
}

func (w *ProtoWriter) Meta(m Meta) (err error) {
	sz := 0
	sz += 1 + varintSize(uint64(len(m.Type))) + len(m.Type)
	for _, l := range m.Data {
		q := len(l)
		sz += 1 + varintSize(uint64(q)) + q
	}

	b, wr := getbuf()
	defer retbuf(b, wr)

	szs := varintSize(uint64(sz))
	b = appendVarint(b, uint64(1+szs+sz))

	b = appendTagVarint(b, 7<<3|2, uint64(sz))

	b = appendTagVarint(b, 1<<3|2, uint64(len(m.Type)))
	b = append(b, m.Type...)

	for _, l := range m.Data {
		b = appendTagVarint(b, 2<<3|2, uint64(len(l)))
		b = append(b, l...)
	}

	_, err = w.w.Write(b)

	return
}

// Message writes enent to the stream.
func (w *ProtoWriter) Message(m Message, sid ID) (err error) {
	b, wr := getbuf()
	defer retbuf(b, wr)

	if m.Location != 0 {
		w.mu.Lock()

		if _, ok := w.ls[m.Location]; !ok {
			defer w.mu.Unlock()

			b = w.location(b, m.Location)
		} else {
			w.mu.Unlock()
		}
	}

	st := len(b)
	switch {
	case m.Args == nil:
		b = append(b, m.Format...)
	case m.Format == "":
		b = AppendPrintln(b, m.Args...)
	default:
		b = AppendPrintf(b, m.Format, m.Args...)
	}
	l := len(b) - st

	sz := 0
	if sid != (ID{}) {
		sz += 1 + varintSize(uint64(len(sid))) + len(sid)
	}
	if m.Location != 0 {
		sz += 1 + varintSize(uint64(m.Location))
	}
	sz += 1 + 8 // m.Time
	sz += 1 + varintSize(uint64(l)) + l

	szs := varintSize(uint64(sz))
	szss := varintSize(uint64(1 + szs + sz))

	total := szss + 1 + szs + sz
	for cap(b) < st+total {
		b = append(b, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	}
	b = b[:st+total]

	copy(b[st+total-l:], b[st:st+l])

	b = appendVarint(b[:st], uint64(1+szs+sz))

	b = appendTagVarint(b, 3<<3|2, uint64(sz))

	if sid != (ID{}) {
		b = appendTagVarint(b, 1<<3|2, uint64(len(sid)))
		b = append(b, sid[:]...)
	}

	if m.Location != 0 {
		b = appendTagVarint(b, 2<<3|0, uint64(m.Location)) //nolint:staticcheck
	}

	b = append(b, 3<<3|1, 0, 0, 0, 0, 0, 0, 0, 0)
	binary.LittleEndian.PutUint64(b[len(b)-8:], uint64(m.Time))

	b = appendTagVarint(b, 4<<3|2, uint64(l))

	// text is already in place
	b = b[:st+total]

	_, err = w.w.Write(b)

	return
}

func (w *ProtoWriter) Metric(m Metric, sid ID) (err error) {
	cnum, had := w.metricCached(m)

	sz := 0
	if sid != (ID{}) {
		sz += 1 + varintSize(uint64(len(sid))) + len(sid)
	}
	sz += 1 + varintSize(uint64(cnum)) // hash
	sz += 1 + 8                        // value
	if !had {
		sz += 1 + varintSize(uint64(len(m.Name))) + len(m.Name)
		for _, l := range m.Labels {
			q := len(l)
			sz += 1 + varintSize(uint64(q)) + q
		}
	}

	b, wr := getbuf()
	defer retbuf(b, wr)

	szs := varintSize(uint64(sz))
	b = appendVarint(b, uint64(1+szs+sz))

	b = appendTagVarint(b, 6<<3|2, uint64(sz))

	if sid != (ID{}) {
		b = appendTagVarint(b, 1<<3|2, uint64(len(sid)))
		b = append(b, sid[:]...)
	}

	b = appendTagVarint(b, 2<<3|0, uint64(cnum)) //nolint:staticcheck

	b = append(b, 3<<3|1, 0, 0, 0, 0, 0, 0, 0, 0)
	binary.LittleEndian.PutUint64(b[len(b)-8:], math.Float64bits(m.Value))

	if !had {
		b = appendTagVarint(b, 4<<3|2, uint64(len(m.Name)))
		b = append(b, m.Name...)

		for _, l := range m.Labels {
			b = appendTagVarint(b, 5<<3|2, uint64(len(l)))
			b = append(b, l...)
		}
	}

	_, err = w.w.Write(b)

	return
}

// SpanStarted writes event to the stream.
func (w *ProtoWriter) SpanStarted(s SpanStart) (err error) {
	sz := 0
	sz += 1 + varintSize(uint64(len(s.ID))) + len(s.ID)
	if s.Parent != (ID{}) {
		sz += 1 + varintSize(uint64(len(s.Parent))) + len(s.Parent)
	}
	if s.Location != 0 {
		sz += 1 + varintSize(uint64(s.Location))
	}
	sz += 1 + 8 // s.Started

	b, wr := getbuf()
	defer retbuf(b, wr)

	if s.Location != 0 {
		w.mu.Lock()

		if _, ok := w.ls[s.Location]; !ok {
			defer w.mu.Unlock()

			b = w.location(b, s.Location)
		} else {
			w.mu.Unlock()
		}
	}

	szs := varintSize(uint64(sz))
	b = appendVarint(b, uint64(1+szs+sz))

	b = appendTagVarint(b, 4<<3|2, uint64(sz))

	b = appendTagVarint(b, 1<<3|2, uint64(len(s.ID)))
	b = append(b, s.ID[:]...)

	if s.Parent != (ID{}) {
		b = appendTagVarint(b, 2<<3|2, uint64(len(s.Parent)))
		b = append(b, s.Parent[:]...)
	}

	if s.Location != 0 {
		b = appendTagVarint(b, 3<<3|0, uint64(s.Location)) //nolint:staticcheck
	}

	b = append(b, 4<<3|1, 0, 0, 0, 0, 0, 0, 0, 0)
	binary.LittleEndian.PutUint64(b[len(b)-8:], uint64(s.Started))

	_, err = w.w.Write(b)

	return
}

// SpanFinished writes event to the stream.
func (w *ProtoWriter) SpanFinished(f SpanFinish) (err error) {
	sz := 0
	sz += 1 + varintSize(uint64(len(f.ID))) + len(f.ID)
	sz += 1 + varintSize(uint64(f.Elapsed))

	b, wr := getbuf()
	defer retbuf(b, wr)

	szs := varintSize(uint64(sz))
	b = appendVarint(b, uint64(1+szs+sz))

	b = appendTagVarint(b, 5<<3|2, uint64(sz))

	b = appendTagVarint(b, 1<<3|2, uint64(len(f.ID)))
	b = append(b, f.ID[:]...)

	b = appendTagVarint(b, 2<<3|0, uint64(f.Elapsed)) //nolint:staticcheck

	_, err = w.w.Write(b)

	return
}

func (w *ProtoWriter) location(b []byte, l Location) []byte {
	name, file, line := l.NameFileLine()

	sz := 0
	sz += 1 + varintSize(uint64(l))
	sz += 1 + varintSize(uint64(l.Entry()))
	sz += 1 + varintSize(uint64(len(name))) + len(name)
	sz += 1 + varintSize(uint64(len(file))) + len(file)
	sz += 1 + varintSize(uint64(line))

	b = appendVarint(b, uint64(1+varintSize(uint64(sz))+sz))

	b = appendTagVarint(b, 2<<3|2, uint64(sz))

	b = appendTagVarint(b, 1<<3|0, uint64(l)) //nolint:staticcheck

	b = appendTagVarint(b, 2<<3|0, uint64(l.Entry())) //nolint:staticcheck

	b = appendTagVarint(b, 3<<3|2, uint64(len(name)))
	b = append(b, name...)

	b = appendTagVarint(b, 4<<3|2, uint64(len(file)))
	b = append(b, file...)

	b = appendTagVarint(b, 5<<3|0, uint64(line)) //nolint:staticcheck

	w.ls[l] = struct{}{}

	return b
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

func (w TeeWriter) Labels(ls Labels, sid ID) (err error) {
	for _, w := range w {
		e := w.Labels(ls, sid)
		if err == nil {
			err = e
		}
	}

	return
}

func (w TeeWriter) Meta(m Meta) (err error) {
	for _, w := range w {
		e := w.Meta(m)
		if err == nil {
			err = e
		}
	}

	return
}

func (w TeeWriter) Message(m Message, sid ID) (err error) {
	for _, w := range w {
		e := w.Message(m, sid)
		if err == nil {
			err = e
		}
	}

	return
}

func (w TeeWriter) Metric(m Metric, sid ID) (err error) {
	for _, w := range w {
		e := w.Metric(m, sid)
		if err == nil {
			err = e
		}
	}

	return
}

func (w TeeWriter) SpanStarted(s SpanStart) (err error) {
	for _, w := range w {
		e := w.SpanStarted(s)
		if err == nil {
			err = e
		}
	}

	return
}

func (w TeeWriter) SpanFinished(f SpanFinish) (err error) {
	for _, w := range w {
		e := w.SpanFinished(f)
		if err == nil {
			err = e
		}
	}

	return
}

func (w DiscardWriter) Labels(Labels, ID) error         { return nil }
func (w DiscardWriter) Meta(Meta) error                 { return nil }
func (w DiscardWriter) Message(Message, ID) error       { return nil }
func (w DiscardWriter) Metric(Metric, ID) error         { return nil }
func (w DiscardWriter) SpanStarted(s SpanStart) error   { return nil }
func (w DiscardWriter) SpanFinished(f SpanFinish) error { return nil }

func NewLockedWriter(w Writer) *LockedWriter {
	return &LockedWriter{w: w}
}

func (w *LockedWriter) Labels(ls Labels, sid ID) error {
	defer w.mu.Unlock()
	w.mu.Lock()

	return w.w.Labels(ls, sid)
}

func (w *LockedWriter) Meta(m Meta) error {
	defer w.mu.Unlock()
	w.mu.Lock()

	return w.w.Meta(m)
}

func (w *LockedWriter) Message(m Message, sid ID) error {
	defer w.mu.Unlock()
	w.mu.Lock()

	return w.w.Message(m, sid)
}

func (w *LockedWriter) Metric(m Metric, sid ID) error {
	defer w.mu.Unlock()
	w.mu.Lock()

	return w.w.Metric(m, sid)
}

func (w *LockedWriter) SpanStarted(s SpanStart) error {
	defer w.mu.Unlock()
	w.mu.Lock()

	return w.w.SpanStarted(s)
}

func (w *LockedWriter) SpanFinished(f SpanFinish) error {
	defer w.mu.Unlock()
	w.mu.Lock()

	return w.w.SpanFinished(f)
}

func NewFallbackWriter(w, fb Writer) FallbackWriter {
	return FallbackWriter{
		Writer:   w,
		Fallback: fb,
	}
}

func (w FallbackWriter) Labels(ls Labels, sid ID) (err error) {
	err = w.Writer.Labels(ls, sid)
	if err != nil {
		_ = w.Fallback.Labels(ls, sid)
	}
	return
}

func (w FallbackWriter) Meta(m Meta) (err error) {
	err = w.Writer.Meta(m)
	if err != nil {
		_ = w.Fallback.Meta(m)
	}
	return
}

func (w FallbackWriter) Message(m Message, sid ID) (err error) {
	err = w.Writer.Message(m, sid)
	if err != nil {
		_ = w.Fallback.Message(m, sid)
	}
	return
}

func (w FallbackWriter) Metric(m Metric, sid ID) (err error) {
	err = w.Writer.Metric(m, sid)
	if err != nil {
		_ = w.Fallback.Metric(m, sid)
	}
	return
}

func (w FallbackWriter) SpanStarted(s SpanStart) (err error) {
	err = w.Writer.SpanStarted(s)
	if err != nil {
		_ = w.Fallback.SpanStarted(s)
	}
	return
}

func (w FallbackWriter) SpanFinished(f SpanFinish) (err error) {
	err = w.Writer.SpanFinished(f)
	if err != nil {
		_ = w.Fallback.SpanFinished(f)
	}
	return
}

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

func LockWriter(w io.Writer) *LockedIOWriter {
	return &LockedIOWriter{w: w}
}

func (w *LockedIOWriter) Write(p []byte) (int, error) {
	defer w.mu.Unlock()
	w.mu.Lock()

	return w.w.Write(p)
}
