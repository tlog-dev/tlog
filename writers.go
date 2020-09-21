package tlog

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

//go:generate protoc --go_out=. tlogpb/tlog.proto

/*
	Wire format

	File (stream, etc) is a series of Records.
	Each Record is one of:

	* Labels - Span Labels or Global if no span id is set (or zero).
	Span Labels take effect for all span messages.
	If the same label key is set with different value it is undefined which will have effect on messages.
	Global Labels take effect on all the following events until next Labels be set.
	If key is not reset in next Labels it is forgotten.

	* Meta - Any metadata. There are few defined by tlog and any can be defined by app. Defined by tlog are:
		* Metric - metric description (name, help message, metric type and metric static labels).

	* Location - Location in code: PC (program counter), file name, line and function name.
	Other events are attached to previously logged locations by PC.
	PC may be reused for another location if it was recompiled.
	So PC life time is limited by file (or stream) it is logged to.

	* Message - Event with timestamp, Location, text and attributes. May be attached to Span or not by its ID.
	Attributes encoded as a list of tuples with name, type and value.

	* Metric - Metric data. Contains Hash, name, value and Labels.
	Hash is calculated somehow from other fields so that they are omitted if was logged earlier it the file (stream).

	* SpanStart - Span started event. Contains ID, Parent ID, timestamp and Location.

	* SpanFinish - Span finished event. Contains ID and time elapsed.
*/

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

		StructuredConfig *StructuredConfig
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

	collectWriter struct {
		Events []cev
	}

	errorWriter struct {
		err error
	}

	cev struct {
		ID ID
		Ev interface{}
	}

	bufWriter []byte

	bwr struct {
		b bufWriter
	}

	awr struct {
		b Attrs
	}

	LockedIOWriter struct {
		mu sync.Mutex
		w  io.Writer
	}
)

var metricsCacheMaxValues = 1000

var ( // type checks
	_ Writer = &ConsoleWriter{}
	_ Writer = &JSONWriter{}
	_ Writer = &ProtoWriter{}
	_ Writer = &TeeWriter{}
	_ Writer = &LockedWriter{}
	_ Writer = FallbackWriter{}
	_ Writer = errorWriter{}
	_ Writer = &collectWriter{}

	Discard Writer = DiscardWriter{}
)

var spaces = []byte("                                                                                                                                                ")

var bufPool = sync.Pool{New: func() interface{} { return &bwr{b: make(bufWriter, 128)} }}

func Getbuf() (_ bufWriter, wr *bwr) { //nolint:golint
	wr = bufPool.Get().(*bwr)
	return wr.b[:0], wr
}

func (wr *bwr) Ret(b *bufWriter) {
	wr.b = *b
	bufPool.Put(wr)
}

var attrsPool = sync.Pool{New: func() interface{} { return &awr{b: make(Attrs, 4)} }}

func GetAttrsbuf() (_ Attrs, wr *awr) { //nolint:golint
	wr = attrsPool.Get().(*awr)
	return wr.b[:0], wr
}

func (wr *awr) Ret(b *Attrs) {
	wr.b = *b
	attrsPool.Put(wr)
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
	b, wr := Getbuf()
	defer wr.Ret(&b)

	b = w.buildHeader(b, m.Time, m.Location)

	if w.f&Lmessagespan != 0 {
		i := len(b)
		b = append(b, "123456789_123456789_123456789_12"[:w.IDWidth]...)
		sid.FormatTo(b[i:i+w.IDWidth], 'v')

		b = append(b, ' ', ' ')
	}

	b = append(b, m.Text...)

	if len(m.Attrs) != 0 {
		b = structuredFormatter(w.StructuredConfig, b, sid, len(m.Text), m.Attrs)
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

	b, wr := Getbuf()
	defer wr.Ret(&b)

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

	b, wr := Getbuf()
	defer wr.Ret(&b)

	b = w.spanHeader(b, f.ID, ID{}, now().UnixNano(), 0)

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

	b, wr := Getbuf()
	defer wr.Ret(&b)

	b = append(b, "Labels:"...)
	for _, l := range ls {
		b = append(b, ' ')
		b = append(b, l...)
	}

	return w.Message(
		Message{
			Location: loc,
			Time:     now().UnixNano(),
			Text:     bytesToString(b),
		},
		sid,
	)
}

func (w *ConsoleWriter) Meta(m Meta) error {
	loc := w.caller()

	b, wr := Getbuf()
	defer wr.Ret(&b)

	b = AppendPrintf(b, "Meta: %v %q", m.Type, m.Data)

	return w.Message(
		Message{
			Location: loc,
			Time:     now().UnixNano(),
			Text:     bytesToString(b),
		},
		ID{},
	)
}

func (w *ConsoleWriter) Metric(m Metric, sid ID) error {
	loc := w.caller()

	b, wr := Getbuf()
	defer wr.Ret(&b)

	b = AppendPrintf(b, "%v  %15.5f ", m.Name, m.Value)

	for _, l := range m.Labels {
		b = append(b, ' ')
		b = append(b, l...)
	}

	return w.Message(
		Message{
			Location: loc,
			Time:     now().UnixNano(),
			Text:     bytesToString(b),
		},
		sid,
	)
}

func (w *ConsoleWriter) caller() Location {
	var buf [6]Location
	FillCallers(2, buf[:])
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
	b, wr := Getbuf()
	defer wr.Ret(&b)

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
	b, wr := Getbuf()
	defer wr.Ret(&b)

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
	b, wr := Getbuf()
	defer wr.Ret(&b)

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

	if m.Time != 0 {
		b = append(b, `"t":`...)
		b = strconv.AppendInt(b, m.Time, 10)
		b = append(b, ',')
	}

	if m.Location != 0 {
		b = append(b, `"l":`...)
		b = strconv.AppendInt(b, int64(m.Location), 10)
		b = append(b, ',')
	}

	b = append(b, `"m":"`...)
	b = append(b, m.Text...)
	b = append(b, '"')

	if len(m.Attrs) != 0 {
		b = w.appendAttrs(b, m.Attrs)
	}

	b = append(b, "}}\n"...)

	_, err = w.w.Write(b)

	return
}

func (w *JSONWriter) appendAttrs(b []byte, attrs Attrs) []byte {
	b = append(b, `,"a":[`...)

	for i, a := range attrs {
		if i != 0 {
			b = append(b, ',')
		}

		b = append(b, `{"n":"`...)
		b = appendSafe(b, a.Name)
		b = append(b, `","t":`...)

		switch v := a.Value.(type) {
		case ID:
			b = append(b, `"d","v":"________________________________"`...)
			v.FormatTo(b[len(b)-1-2*len(v):], 'x')
		case string:
			b = append(b, `"s","v":"`...)
			b = appendSafe(b, v)
			b = append(b, '"')
		case int, int64, int32, int16, int8:
			b = append(b, `"i","v":`...)

			var iv int64
			switch v := v.(type) {
			case int:
				iv = int64(v)
			case int64:
				iv = int64(v) //nolint:unconvert
			case int32:
				iv = int64(v)
			case int16:
				iv = int64(v)
			case int8:
				iv = int64(v)
			}

			b = strconv.AppendInt(b, iv, 10)
		case uint, uint64, uint32, uint16, uint8:
			b = append(b, `"u","v":`...)

			var iv uint64
			switch v := v.(type) {
			case uint:
				iv = uint64(v)
			case uint64:
				iv = uint64(v) //nolint:unconvert
			case uint32:
				iv = uint64(v)
			case uint16:
				iv = uint64(v)
			case uint8:
				iv = uint64(v)
			}

			b = strconv.AppendUint(b, iv, 10)
		case float64:
			b = append(b, `"f","v":`...)
			b = strconv.AppendFloat(b, v, 'f', -1, 64)
		case float32:
			b = append(b, `"f","v":`...)
			b = strconv.AppendFloat(b, float64(v), 'f', -1, 32)
		default:
			b = AppendPrintf(b, `"?","ut":"%T"`, v)
		}

		b = append(b, '}')
	}

	b = append(b, ']')

	return b
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

func (w *writerCache) metricCached(m Metric) (cnum int, full bool) {
	skip := w.skip[m.Name]
	if skip == -1 {
		return 0, true
	}

	if skip > metricsCacheMaxValues {
		w.killMetric(m.Name)

		return 0, true
	}

	n := m.Name
	h := strhash(&n, 0)
	for _, l := range m.Labels {
		n = l
		h = strhash(&n, h)
	}

	cnum = -1
	full = false
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

		break
	}

	if cnum != -1 {
		return
	}

	w.ccn++
	cnum = w.ccn
	full = true

	w.skip[m.Name]++

	ls := make(Labels, len(m.Labels))
	copy(ls, m.Labels)

	w.cc[h] = append(w.cc[h], mh{
		N:      cnum,
		Name:   m.Name,
		Labels: ls,
	})

	return
}

func (w *JSONWriter) Metric(m Metric, sid ID) (err error) {
	w.mu.Lock()
	cnum, full := w.metricCached(m)
	w.mu.Unlock()

	b, wr := Getbuf()
	defer wr.Ret(&b)

	b = append(b, `{"v":{`...)

	if sid != (ID{}) {
		b = append(b, `"s":"`...)
		i := len(b)
		b = append(b, `123456789_123456789_123456789_12",`...)
		sid.FormatTo(b[i:], 'x')
	}

	if cnum != 0 {
		b = append(b, `"h":`...)
		b = strconv.AppendInt(b, int64(cnum), 10)
		b = append(b, ',')
	}

	b = append(b, `"v":`...)
	b = strconv.AppendFloat(b, m.Value, 'g', -1, 64)

	if full {
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
	b, wr := Getbuf()
	defer wr.Ret(&b)

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
	b, wr := Getbuf()
	defer wr.Ret(&b)

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

	b, wr := Getbuf()
	defer wr.Ret(&b)

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

	b, wr := Getbuf()
	defer wr.Ret(&b)

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
	b, wr := Getbuf()
	defer wr.Ret(&b)

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
	l := len(m.Text)

	sz := 0
	if sid != (ID{}) {
		sz += 1 + varintSize(uint64(len(sid))) + len(sid)
	}
	if m.Location != 0 {
		sz += 1 + varintSize(uint64(m.Location))
	}
	if m.Time != 0 {
		sz += 1 + 8 // m.Time
	}
	if l != 0 {
		sz += 1 + varintSize(uint64(l)) + l
	}
	if len(m.Attrs) != 0 {
		for _, a := range m.Attrs {
			as := w.attrSize(a)
			sz += 1 + varintSize(uint64(as)) + as
		}
	}

	szs := varintSize(uint64(sz))

	b = appendVarint(b[:st], uint64(1+szs+sz))

	b = appendTagVarint(b, 3<<3|2, uint64(sz))

	if sid != (ID{}) {
		b = appendTagVarint(b, 1<<3|2, uint64(len(sid)))
		b = append(b, sid[:]...)
	}

	if m.Location != 0 {
		b = appendTagVarint(b, 2<<3|0, uint64(m.Location)) //nolint:staticcheck
	}

	if m.Time != 0 {
		b = append(b, 3<<3|1, 0, 0, 0, 0, 0, 0, 0, 0)
		binary.LittleEndian.PutUint64(b[len(b)-8:], uint64(m.Time))
	}

	if l != 0 {
		b = appendTagVarint(b, 4<<3|2, uint64(l))
		b = append(b, m.Text...)
	}

	if len(m.Attrs) != 0 {
		b = w.appendAttrs(b, m.Attrs)
	}

	_, err = w.w.Write(b)

	return
}

func (w *ProtoWriter) attrSize(a Attr) (s int) {
	s = 1 + varintSize(uint64(len(a.Name))) + len(a.Name)

	switch v := a.Value.(type) {
	case ID:
		s += 1 + 1
		s += 1 + 1 + len(v)
	case string:
		s += 1 + 1
		s += 1 + varintSize(uint64(len(v))) + len(v)
	case int, int64, int32, int16, int8:
		s += 1 + 1
		switch v := v.(type) {
		case int:
			s += 1 + varintSize(uint64(v))
		case int64:
			s += 1 + varintSize(uint64(v))
		case int32:
			s += 1 + varintSize(uint64(v))
		case int16:
			s += 1 + varintSize(uint64(v))
		case int8:
			s += 1 + varintSize(uint64(v))
		}
	case uint, uint64, uint32, uint16, uint8:
		var tv uint64
		switch v := v.(type) {
		case uint:
			tv = uint64(v)
		case uint64:
			tv = uint64(v) // nolint:unconvert
		case uint32:
			tv = uint64(v)
		case uint16:
			tv = uint64(v)
		case uint8:
			tv = uint64(v)
		}

		s += 1 + 1
		s += 1 + varintSize(tv)
	case float64, float32:
		s += 1 + 1
		s += 1 + 8
	default:
		tp := fmt.Sprintf("%T", v)

		s += 1 + 1
		s += 1 + varintSize(uint64(len(tp))) + len(tp)
	}

	return
}

//nolint:staticcheck
func (w *ProtoWriter) appendAttrs(b []byte, attrs Attrs) []byte {
	for _, a := range attrs {
		as := w.attrSize(a)

		b = appendTagVarint(b, 5<<3|2, uint64(as))

		b = appendTagVarint(b, 1<<3|2, uint64(len(a.Name)))
		b = append(b, a.Name...)

		switch v := a.Value.(type) {
		case ID:
			b = appendTagVarint(b, 2<<3|0, uint64('d'))

			b = appendTagVarint(b, 7<<3|2, uint64(len(v)))
			b = append(b, v[:]...)
		case string:
			b = appendTagVarint(b, 2<<3|0, uint64('s'))

			b = appendTagVarint(b, 3<<3|2, uint64(len(v)))
			b = append(b, v...)
		case int, int64, int32, int16, int8:
			b = appendTagVarint(b, 2<<3|0, uint64('i'))

			var tv uint64
			switch v := v.(type) {
			case int:
				tv = uint64(v)
			case int64:
				tv = uint64(v)
			case int32:
				tv = uint64(v)
			case int16:
				tv = uint64(v)
			case int8:
				tv = uint64(v)
			}

			b = appendTagVarint(b, 4<<3|0, tv)
		case uint, uint64, uint32, uint16, uint8:
			b = appendTagVarint(b, 2<<3|0, uint64('u'))

			var tv uint64
			switch v := v.(type) {
			case uint:
				tv = uint64(v)
			case uint64:
				tv = uint64(v) //nolint:unconvert
			case uint32:
				tv = uint64(v)
			case uint16:
				tv = uint64(v)
			case uint8:
				tv = uint64(v)
			}

			b = appendTagVarint(b, 5<<3|0, tv)
		case float64:
			b = appendTagVarint(b, 2<<3|0, uint64('f'))

			b = append(b, 6<<3|1, 0, 0, 0, 0, 0, 0, 0, 0)
			binary.LittleEndian.PutUint64(b[len(b)-8:], math.Float64bits(v))
		case float32:
			b = appendTagVarint(b, 2<<3|0, uint64('f'))

			b = append(b, 6<<3|1, 0, 0, 0, 0, 0, 0, 0, 0)
			binary.LittleEndian.PutUint64(b[len(b)-8:], math.Float64bits(float64(v)))
		default:
			tp := fmt.Sprintf("%T", v)

			b = appendTagVarint(b, 2<<3|0, uint64('?'))

			b = appendTagVarint(b, 3<<3|2, uint64(len(tp)))
			b = append(b, tp...)
		}
	}

	return b
}

func (w *ProtoWriter) Metric(m Metric, sid ID) (err error) {
	cnum, full := w.metricCached(m)

	sz := 0
	if sid != (ID{}) {
		sz += 1 + varintSize(uint64(len(sid))) + len(sid)
	}
	if cnum != 0 {
		sz += 1 + varintSize(uint64(cnum)) // hash
	}
	sz += 1 + 8 // value
	if full {
		sz += 1 + varintSize(uint64(len(m.Name))) + len(m.Name)
		for _, l := range m.Labels {
			q := len(l)
			sz += 1 + varintSize(uint64(q)) + q
		}
	}

	b, wr := Getbuf()
	defer wr.Ret(&b)

	szs := varintSize(uint64(sz))
	b = appendVarint(b, uint64(1+szs+sz))

	b = appendTagVarint(b, 6<<3|2, uint64(sz))

	if sid != (ID{}) {
		b = appendTagVarint(b, 1<<3|2, uint64(len(sid)))
		b = append(b, sid[:]...)
	}

	if cnum != 0 {
		b = appendTagVarint(b, 2<<3|0, uint64(cnum)) //nolint:staticcheck
	}

	b = append(b, 3<<3|1, 0, 0, 0, 0, 0, 0, 0, 0)
	binary.LittleEndian.PutUint64(b[len(b)-8:], math.Float64bits(m.Value))

	if full {
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

	b, wr := Getbuf()
	defer wr.Ret(&b)

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

	b, wr := Getbuf()
	defer wr.Ret(&b)

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

func (w errorWriter) Labels(Labels, ID) error         { return w.err }
func (w errorWriter) Meta(Meta) error                 { return w.err }
func (w errorWriter) Message(Message, ID) error       { return w.err }
func (w errorWriter) Metric(Metric, ID) error         { return w.err }
func (w errorWriter) SpanStarted(s SpanStart) error   { return w.err }
func (w errorWriter) SpanFinished(f SpanFinish) error { return w.err }

func (w *collectWriter) Labels(ls Labels, id ID) error {
	w.Events = append(w.Events, cev{Ev: ls, ID: id})
	return nil
}

func (w *collectWriter) Meta(m Meta) error {
	w.Events = append(w.Events, cev{Ev: m})
	return nil
}

func (w *collectWriter) Message(m Message, id ID) error {
	w.Events = append(w.Events, cev{Ev: m, ID: id})
	return nil
}

func (w *collectWriter) Metric(m Metric, id ID) error {
	w.Events = append(w.Events, cev{Ev: m, ID: id})
	return nil
}

func (w *collectWriter) SpanStarted(s SpanStart) error {
	w.Events = append(w.Events, cev{Ev: s})
	return nil
}

func (w *collectWriter) SpanFinished(f SpanFinish) error {
	w.Events = append(w.Events, cev{Ev: f})
	return nil
}

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

type CountableIODiscard struct {
	B, N int64
}

func (w *CountableIODiscard) ReportDisk(b *testing.B) {
	b.ReportMetric(float64(w.B)/float64(b.N), "disk_B/op")
}

func (w *CountableIODiscard) Write(p []byte) (int, error) {
	atomic.AddInt64(&w.N, 1)
	atomic.AddInt64(&w.B, int64(len(p)))

	return len(p), nil
}
