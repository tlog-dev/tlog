package tlog

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh/terminal"
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
		* Metric - metric description (name, help message, metric type).

	* Location - Location in code: PC (program counter), file name, line and function name.
	Other events are attached to previously logged locations by PC.
	PC may be reused for another location if it was recompiled.
	So PC life time is limited by file (or stream) it is logged to.

	* Message - Event with timestamp, PC, text and attributes. May be attached to Span or not by its ID.
	Attributes encoded as a list of tuples with name, type and value.

	* Metric - Metric data. Contains name, value and Labels.

	* SpanStart - Span started event. Contains ID, Parent ID, timestamp and PC.

	* SpanFinish - Span finished event. Contains ID and time elapsed.
*/

type (
	// ConsoleWriter produces similar output as stdlib log.Logger.
	//
	// Each event ends up with a single Write.
	ConsoleWriter struct {
		w io.Writer
		f int

		// column widths
		Shortfile  int
		Funcname   int
		IDWidth    int
		LevelWidth int

		Colorize bool

		ColorConfig      *ColorConfig
		StructuredConfig *StructuredConfig
	}

	ColorConfig struct {
		Time       int
		File       int
		Func       int
		SpanID     int
		Message    int
		AttrKey    int
		AttrValue  int
		DebugLevel int
		Levels     [4]int
	}

	// JSONWriter produces output readable by both machines and humans.
	//
	// Each event ends up with a single Write if message fits in 1000 bytes (default) buffer.
	JSONWriter struct {
		w io.Writer

		mu sync.RWMutex
		ls map[PC]struct{}
	}

	// ProtoWriter encodes event logs in protobuf and produces more compact output then JSONWriter.
	//
	// Each event ends up with a single Write.
	ProtoWriter struct {
		w io.Writer

		mu sync.RWMutex
		ls map[PC]struct{}
	}

	// TeeWriter writes the same events in the same order to all Writers one after another.
	TeeWriter []Writer

	// DiscardWriter discards all events.
	DiscardWriter struct{}

	// LockedWriter is a Writer under Mutex.
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

	errorWriter struct {
		err error
	}

	collectWriter struct {
		Events []cev
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

	// LockedIOWriter is io.Writer protected by sync.Mutex.
	LockedIOWriter struct {
		mu sync.Mutex
		w  io.Writer
	}

	// CountableIODiscard discards data but counts operations and bytes.
	// It's safe to use simultaneously (atimic operations are used).
	CountableIODiscard struct {
		B, N int64
	}

	BufferedWriter struct {
		io.Writer
		mu sync.Mutex
		b  []byte
	}
)

const ( // colors
	ColorBlack = iota + 30
	ColorRed
	ColorGreen
	ColorYellow
	ColorBlue
	ColorMagenta
	ColorCyan
	ColorWhite

	ColorDefault  = 0
	ColorBold     = 1
	ColorDarkGray = 90
)

var DefaultColorConfig = ColorConfig{
	Time:       ColorDarkGray,
	File:       ColorDarkGray,
	Func:       ColorDarkGray,
	SpanID:     ColorDarkGray,
	DebugLevel: ColorDarkGray,
	Levels: [4]int{
		ColorDarkGray,
		ColorYellow,
		ColorRed,
		ColorBlue,
	},
	AttrKey: ColorCyan,
}

var colors [256][]byte

func init() {
	for i := range colors {
		switch {
		case i < 10:
			colors[i] = []byte{'\x1b', '[', '0' + byte(i%10), 'm'}
		case i < 100:
			colors[i] = []byte{'\x1b', '[', '0' + byte(i/10), '0' + byte(i%10), 'm'}
		default:
			colors[i] = []byte{'\x1b', '[', '0' + byte(i/100), '0' + byte(i/10%10), '0' + byte(i%10), 'm'}
		}
	}
}

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

	// Discard is a writer that discards all events.
	Discard Writer = DiscardWriter{}
)

var spaces = []byte("                                                                                                                                                ")

var metricsTest bool

var bufPool = sync.Pool{New: func() interface{} { return &bwr{b: make(bufWriter, 128)} }}

// Getbuf gets bytes buffer from a pool to reduce gc pressure.
// Buffer must be returned after used. Usage:
//     b, wr := tlog.Getbuf()
//     defer wr.Ret(&b)
//
//     b = append(b[:0], ...)
func Getbuf() (_ bufWriter, wr *bwr) { //nolint:golint
	wr = bufPool.Get().(*bwr)
	return wr.b[:0], wr
}

func (wr *bwr) Ret(b *bufWriter) {
	wr.b = *b
	bufPool.Put(wr)
}

var attrsPool = sync.Pool{New: func() interface{} { return &awr{b: make(Attrs, 4)} }}

// GetAttrsbuf gets Attrs buffer from a pool to reduce gc pressure.
// Buffer must be returned after used. Usage:
//     b, wr := tlog.GetAttrsbuf()
//     defer wr.Ret(&b)
//
//     b = append(b[:0], ...)
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
	var colorize bool
	if f, ok := w.(*os.File); ok {
		colorize = terminal.IsTerminal(int(f.Fd()))
	}

	return &ConsoleWriter{
		w:          w,
		f:          f,
		Shortfile:  20,
		Funcname:   18,
		IDWidth:    8,
		LevelWidth: 3,
		Colorize:   colorize,
	}
}

//nolint:gocognit,gocyclo,nestif
func (w *ConsoleWriter) buildHeader(b []byte, lv Level, ts int64, loc PC) []byte {
	var fname, file string
	line := -1

	var col *ColorConfig
	if w.Colorize {
		col = w.ColorConfig
		if col == nil {
			col = &DefaultColorConfig
		}
	}

	if w.f&(Ldate|Ltime|Lmilliseconds|Lmicroseconds) != 0 {
		t := time.Unix(0, ts)

		if w.f&LUTC != 0 {
			t = t.UTC()
		}

		var Y, M, D, h, m, s int
		if w.f&(Ldate|Ltime) != 0 {
			Y, M, D, h, m, s = splitTime(t)
		}

		if col != nil && col.Time != 0 {
			b = append(b, colors[col.Time]...)
		}

		if w.f&Ldate != 0 {
			i := len(b)
			b = append(b, "0000-00-00"...)

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

		if col != nil && col.Time != 0 {
			b = append(b, colors[0]...)
		}

		b = append(b, ' ', ' ')
	}

	if w.f&Llevel != 0 {
		var color int
		if col != nil {
			switch {
			case lv < 0:
				color = col.DebugLevel
			case lv > FatalLevel:
				color = col.Levels[FatalLevel]
			default:
				color = col.Levels[lv]
			}
		}

		if color != 0 {
			b = append(b, colors[color]...)
		}

		i := len(b)
		b = append(b, spaces[:w.LevelWidth]...)

		switch lv {
		case InfoLevel:
			copy(b[i:], "INFO")
		case WarnLevel:
			copy(b[i:], "WARN")
		case ErrorLevel:
			copy(b[i:], "ERROR")
		case FatalLevel:
			copy(b[i:], "FATAL")
		default:
			b = strconv.AppendInt(b[i:], int64(lv), 16)
		}

		end := len(b)

		if color != 0 {
			b = append(b, colors[0]...)
		}

		if pad := i + w.LevelWidth + 2 - end; pad > 0 {
			b = append(b, spaces[:pad]...)
		}
	}

	if w.f&(Llongfile|Lshortfile) != 0 {
		fname, file, line = loc.NameFileLine()

		if col != nil && col.File != 0 {
			b = append(b, colors[col.File]...)
		}

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

		if col != nil && col.File != 0 {
			b = append(b, colors[0]...)
		}

		b = append(b, ' ', ' ')
	}

	if w.f&(Ltypefunc|Lfuncname) != 0 {
		if line == -1 {
			fname, _, _ = loc.NameFileLine()
		}
		fname = filepath.Base(fname)

		if col != nil && col.Func != 0 {
			b = append(b, colors[col.Func]...)
		}

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

		if col != nil && col.Func != 0 {
			b = append(b, colors[0]...)
		}

		b = append(b, ' ', ' ')
	}

	return b
}

// Message writes Message event by single Write.
func (w *ConsoleWriter) Message(m Message, sid ID) (err error) {
	b, wr := Getbuf()
	defer wr.Ret(&b)

	if w.f&Lmessagespan != 0 {
		b = w.spanHeader(b, sid, m.Level, m.Time, m.PC)
	} else {
		b = w.buildHeader(b, m.Level, m.Time, m.PC)
	}

	var color int
	if w.Colorize {
		if w.ColorConfig != nil {
			color = w.ColorConfig.Message
		} else {
			color = DefaultColorConfig.Message
		}

		if color != 0 {
			b = append(b, colors[color]...)
		}
	}

	b = append(b, m.Text...)

	if color != 0 {
		b = append(b, colors[0]...)
	}

	if len(m.Attrs) != 0 {
		b = structuredFormatter(w, b, sid, len(m.Text), m.Attrs)
	}

	b.NewLine()

	_, err = w.w.Write(b)

	return
}

func (w *ConsoleWriter) spanHeader(b []byte, sid ID, lv Level, tm int64, loc PC) []byte {
	b = w.buildHeader(b, lv, tm, loc)

	var color int
	if w.Colorize {
		if w.ColorConfig != nil {
			color = w.ColorConfig.SpanID
		} else {
			color = DefaultColorConfig.SpanID
		}

		if color != 0 {
			b = append(b, colors[color]...)
		}
	}

	i := len(b)
	b = append(b, "123456789_123456789_123456789_12"[:w.IDWidth]...)
	sid.FormatTo(b[i:i+w.IDWidth], 'v')

	if color != 0 {
		b = append(b, colors[0]...)
	}

	return append(b, ' ', ' ')
}

// SpanStarted writes SpanStarted event by single Write.
func (w *ConsoleWriter) SpanStarted(s SpanStart) (err error) {
	if w.f&Lspans == 0 {
		return
	}

	b, wr := Getbuf()
	defer wr.Ret(&b)

	b = w.spanHeader(b, s.ID, 0, s.StartedAt, s.PC)

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

	b = w.spanHeader(b, f.ID, 0, now().UnixNano(), 0)

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
			PC:   loc,
			Time: now().UnixNano(),
			Text: bytesToString(b),
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
			PC:   loc,
			Time: now().UnixNano(),
			Text: bytesToString(b),
		},
		ID{},
	)
}

func (w *ConsoleWriter) Metric(m Metric, sid ID) error {
	loc := w.caller()

	b, wr := Getbuf()
	defer wr.Ret(&b)

	wh := DefaultStructuredConfig.MessageWidth
	if cfg := w.StructuredConfig; cfg != nil {
		wh = cfg.MessageWidth
	}

	b = AppendPrintf(b, "%-*v  %15.5f ", wh, m.Name, m.Value)

	for _, l := range m.Labels {
		b = append(b, ' ')
		b = append(b, l...)
	}

	return w.Message(
		Message{
			PC:   loc,
			Time: now().UnixNano(),
			Text: bytesToString(b),
		},
		sid,
	)
}

func (w *ConsoleWriter) caller() PC {
	var buf [6]PC
	CallersFill(2, buf[:])
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

// NewJSONWriter creates JSON writer.
func NewJSONWriter(w io.Writer) *JSONWriter {
	return &JSONWriter{
		w:  w,
		ls: make(map[PC]struct{}),
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

	if m.PC != 0 {
		w.mu.RLock()
		_, ok := w.ls[m.PC]
		w.mu.RUnlock()

		if !ok {
			w.mu.Lock()
			defer w.mu.Unlock()

			b = w.location(b, m.PC)
		}
	}

	var comma bool

	b = append(b, `{"m":{`...)

	if sid != (ID{}) {
		b = append(b, `"s":"`...)
		i := len(b)
		b = append(b, `123456789_123456789_123456789_12"`...)
		sid.FormatTo(b[i:], 'x')

		comma = true
	}

	if m.Time != 0 {
		if comma {
			b = append(b, ',')
		}

		b = append(b, `"t":`...)
		b = strconv.AppendInt(b, m.Time, 10)

		comma = true
	}

	if m.Level != 0 {
		if comma {
			b = append(b, ',')
		}

		b = append(b, `"i":`...)
		b = strconv.AppendInt(b, int64(m.Level), 10)

		comma = true
	}

	if m.PC != 0 {
		if comma {
			b = append(b, ',')
		}

		b = append(b, `"l":`...)
		b = strconv.AppendInt(b, int64(m.PC), 10)

		comma = true
	}

	if m.Text != "" {
		if comma {
			b = append(b, ',')
		}

		b = append(b, `"m":"`...)
		b = append(b, m.Text...)
		b = append(b, '"')

		comma = true
	}

	if len(m.Attrs) != 0 {
		if comma {
			b = append(b, ',')
		}

		b = w.appendAttrs(b, m.Attrs)
	}

	b = append(b, "}}\n"...)

	_, err = w.w.Write(b)

	return
}

func (w *JSONWriter) appendAttrs(b []byte, attrs Attrs) []byte {
	b = append(b, `"a":[`...)

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
		case fmt.Stringer:
			val := v.String()

			b = append(b, `"s","v":"`...)
			b = appendSafe(b, val)
			b = append(b, '"')
		default:
			b = AppendPrintf(b, `"?","v":"%T"`, v)
		}

		b = append(b, '}')
	}

	b = append(b, ']')

	return b
}

func (w *JSONWriter) Metric(m Metric, sid ID) (err error) {
	b, wr := Getbuf()
	defer wr.Ret(&b)

	b = append(b, `{"v":{`...)

	if sid != (ID{}) {
		b = append(b, `"s":"`...)
		i := len(b)
		b = append(b, `123456789_123456789_123456789_12",`...)
		sid.FormatTo(b[i:], 'x')
	}

	b = append(b, `"n":"`...)
	b = appendSafe(b, m.Name)
	b = append(b, '"')

	b = append(b, `,"v":`...)
	b = strconv.AppendFloat(b, m.Value, 'g', -1, 64)

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

	b = append(b, `}}`+"\n"...)

	_, err = w.w.Write(b)

	return
}

// SpanStarted writes event to the stream.
func (w *JSONWriter) SpanStarted(s SpanStart) (err error) {
	b, wr := Getbuf()
	defer wr.Ret(&b)

	if s.PC != 0 {
		w.mu.RLock()
		_, ok := w.ls[s.PC]
		w.mu.RUnlock()

		if !ok {
			w.mu.Lock()
			defer w.mu.Unlock()

			b = w.location(b, s.PC)
		}
	}

	b = append(b, `{"s":{"i":"`...)
	i := len(b)
	b = append(b, `123456789_123456789_123456789_12"`...)
	s.ID.FormatTo(b[i:], 'x')

	b = append(b, `,"s":`...)
	b = strconv.AppendInt(b, s.StartedAt, 10)

	b = append(b, `,"l":`...)
	b = strconv.AppendInt(b, int64(s.PC), 10)

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

func (w *JSONWriter) location(b []byte, l PC) []byte {
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

// NewProtoWriter creates protobuf writer.
func NewProtoWriter(w io.Writer) *ProtoWriter {
	return &ProtoWriter{
		w:  w,
		ls: make(map[PC]struct{}),
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

	if m.PC != 0 {
		w.mu.RLock()
		_, ok := w.ls[m.PC]
		w.mu.RUnlock()

		if !ok {
			w.mu.Lock()
			defer w.mu.Unlock()

			b = w.location(b, m.PC)
		}
	}

	st := len(b)
	l := len(m.Text)

	sz := 0
	if sid != (ID{}) {
		sz += 1 + varintSize(uint64(len(sid))) + len(sid)
	}
	if m.PC != 0 {
		sz += 1 + varintSize(uint64(m.PC))
	}
	if m.Time != 0 {
		sz += 1 + 8 // m.Time
	}
	if l != 0 {
		sz += 1 + varintSize(uint64(l)) + l
	}
	if m.Level != 0 {
		sz += 1 + varintSize(uint64(int64(m.Level)<<1)^uint64(int64(m.Level)>>63))
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

	if m.PC != 0 {
		b = appendTagVarint(b, 2<<3|0, uint64(m.PC)) //nolint:staticcheck
	}

	if m.Time != 0 {
		b = append(b, 3<<3|1, 0, 0, 0, 0, 0, 0, 0, 0)
		binary.LittleEndian.PutUint64(b[len(b)-8:], uint64(m.Time))
	}

	if m.Level != 0 {
		lv := uint64(int64(m.Level)<<1) | uint64(int64(m.Level)>>63)
		b = appendTagVarint(b, 4<<3|0, lv) //nolint:staticcheck
	}

	if l != 0 {
		b = appendTagVarint(b, 5<<3|2, uint64(l))
		b = append(b, m.Text...)
	}

	if len(m.Attrs) != 0 {
		b = w.appendAttrs(b, m.Attrs)
	}

	_, err = w.w.Write(b)

	return
}

func (w *ProtoWriter) attrSize(a Attr) (s int) {
	s = 1 + 1
	s += 1 + varintSize(uint64(len(a.Name))) + len(a.Name)

	switch v := a.Value.(type) {
	case ID:
		s += 1 + 1 + len(v)
	case string:
		s += 1 + varintSize(uint64(len(v))) + len(v)
	case int, int64, int32, int16, int8:
		var tv int64
		switch v := v.(type) {
		case int:
			tv = int64(v)
		case int64:
			tv = int64(v) // nolint:unconvert
		case int32:
			tv = int64(v)
		case int16:
			tv = int64(v)
		case int8:
			tv = int64(v)
		}

		tvu := uint64(tv<<1) ^ uint64(tv>>63)

		s += 1 + varintSize(tvu)
	case uint, uint64, uint32, uint16, uint8:
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

		s += 1 + varintSize(tv)
	case float64, float32:
		s += 1 + 8
	default:
		tp := fmt.Sprintf("%T", v)

		s += 1 + varintSize(uint64(len(tp))) + len(tp)
	}

	return
}

//nolint:staticcheck
func (w *ProtoWriter) appendAttrs(b []byte, attrs Attrs) []byte {
	for _, a := range attrs {
		as := w.attrSize(a)

		b = appendTagVarint(b, 6<<3|2, uint64(as))

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

			var tv int64
			switch v := v.(type) {
			case int:
				tv = int64(v)
			case int64:
				tv = int64(v) //nolint:unconvert
			case int32:
				tv = int64(v)
			case int16:
				tv = int64(v)
			case int8:
				tv = int64(v)
			}

			tvu := uint64(tv<<1) ^ uint64(tv>>63)

			b = appendTagVarint(b, 4<<3|0, tvu)
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
		case fmt.Stringer: // encode as string
			b = appendTagVarint(b, 2<<3|0, uint64('s'))

			val := v.String()

			b = appendTagVarint(b, 3<<3|2, uint64(len(val)))
			b = append(b, val...)
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
	sz := 0
	if sid != (ID{}) {
		sz += 1 + varintSize(uint64(len(sid))) + len(sid)
	}
	sz += 1 + 8 // value
	sz += 1 + varintSize(uint64(len(m.Name))) + len(m.Name)
	for _, l := range m.Labels {
		q := len(l)
		sz += 1 + varintSize(uint64(q)) + q
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

	b = appendTagVarint(b, 2<<3|2, uint64(len(m.Name)))
	b = append(b, m.Name...)

	b = append(b, 3<<3|1, 0, 0, 0, 0, 0, 0, 0, 0)
	binary.LittleEndian.PutUint64(b[len(b)-8:], math.Float64bits(m.Value))

	for _, l := range m.Labels {
		b = appendTagVarint(b, 4<<3|2, uint64(len(l)))
		b = append(b, l...)
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
	if s.PC != 0 {
		sz += 1 + varintSize(uint64(s.PC))
	}
	sz += 1 + 8 // s.StartedAt

	b, wr := Getbuf()
	defer wr.Ret(&b)

	if s.PC != 0 {
		w.mu.RLock()
		_, ok := w.ls[s.PC]
		w.mu.RUnlock()

		if !ok {
			w.mu.Lock()
			defer w.mu.Unlock()

			b = w.location(b, s.PC)
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

	if s.PC != 0 {
		b = appendTagVarint(b, 3<<3|0, uint64(s.PC)) //nolint:staticcheck
	}

	b = append(b, 4<<3|1, 0, 0, 0, 0, 0, 0, 0, 0)
	binary.LittleEndian.PutUint64(b[len(b)-8:], uint64(s.StartedAt))

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

func (w *ProtoWriter) location(b []byte, l PC) []byte {
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

func (w *CountableIODiscard) ReportDisk(b *testing.B) {
	b.ReportMetric(float64(w.B)/float64(b.N), "disk_B/op")
}

func (w *CountableIODiscard) Write(p []byte) (int, error) {
	atomic.AddInt64(&w.N, 1)
	atomic.AddInt64(&w.B, int64(len(p)))

	return len(p), nil
}

func (w *BufferedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.b = append(w.b, p...)
	w.mu.Unlock()

	return len(p), nil
}

func (w *BufferedWriter) Flush() (err error) {
	defer w.mu.Unlock()
	w.mu.Lock()

	_, err = w.Writer.Write(w.b)

	w.b = w.b[:0]

	return
}
