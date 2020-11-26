package writer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh/terminal"

	"github.com/nikandfor/tlog/core"
	"github.com/nikandfor/tlog/low"
	"github.com/nikandfor/tlog/wire"
)

type (
	ID    = core.ID
	Type  = core.Type
	Level = core.Level

	Console struct {
		w io.Writer
		f int

		d wire.Decoder

		// column widths
		LabelsHash int
		Shortfile  int
		Funcname   int
		IDWidth    int
		LevelWidth int

		Colorize bool

		ColorConfig      *ColorConfig
		StructuredConfig *StructuredConfig

		colKey, colVal []byte

		b low.Buf
	}

	ColorConfig struct {
		Time      int
		File      int
		Func      int
		SpanID    int
		Message   int
		AttrKey   int
		AttrValue int
		Debug     int
		Levels    [4]int
	}

	StructuredConfig struct {
		// Minimal message width
		MessageWidth     int
		ValueMaxPadWidth int

		PairSeparator string
		KVSeparator   string

		QuoteAnyValue   bool
		QuoteEmptyValue bool

		structValWidth sync.Map // string -> int
	}

	location struct {
		Name, File string
		Line       int
	}
)

const (
	Info = iota
	Warn
	Error
	Fatal

	Debug = -1
)

// Console flags. Similar to log.Logger flags.
const ( // console writer flags
	Ldate = 1 << iota
	Ltime
	Lseconds
	Lmilliseconds
	Lmicroseconds
	Llevel
	Lshortfile
	Llongfile
	Ltypefunc // pkg.(*Type).Func
	Lfuncname // Func
	LUTC
	Lspans       // print Span start and finish events
	Lmessagespan // add Span ID to trace messages
	LstdFlags    = Ldate | Ltime
	LdetFlags    = Ldate | Ltime | Llevel | Lmicroseconds | Lshortfile
	Lnone        = 0
)

// DefaultStructuredConfig is default config to format structured logs by Console writer.
var DefaultStructuredConfig = StructuredConfig{
	MessageWidth:     30,
	ValueMaxPadWidth: 24,
	PairSeparator:    "  ",
	KVSeparator:      "=",
}

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
	Time:   ColorDarkGray,
	File:   ColorDarkGray,
	Func:   ColorDarkGray,
	SpanID: ColorDarkGray,
	Debug:  ColorDarkGray,
	Levels: [4]int{
		ColorDarkGray,
		ColorRed,
		ColorRed,
		ColorRed,
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

// NewConsoleWriter creates writer with similar output as log.Logger.
func NewConsole(w io.Writer, f int) *Console {
	var colorize bool
	switch f := w.(type) {
	case interface {
		Fd() uintptr
	}:
		colorize = terminal.IsTerminal(int(f.Fd()))
	case interface {
		Fd() int
	}:
		colorize = terminal.IsTerminal(f.Fd())
	}

	return &Console{
		w:          w,
		f:          f,
		LabelsHash: 3,
		Shortfile:  20,
		Funcname:   18,
		IDWidth:    8,
		LevelWidth: 3,
		Colorize:   colorize,
	}
}

func (w *Console) SetFlags(f int) {
	w.f = f
}

func (w *Console) Write(p []byte) (i int, err error) {
	var ev wire.EventHeader

	defer func() {
		perr := recover()
		if perr == nil {
			return
		}

		fmt.Fprintf(os.Stderr, "panic: %v\non message\n%v\nstack:\n%s\n", perr, wire.Dump(p), debug.Stack())
	}()

	//	defer func() {
	//		fmt.Fprintf(os.Stderr, "console.Write %v %v\n", i, err)
	//	}()

	i, err = w.d.DecodeHeader(p, &ev)
	if err != nil {
		return 0, err
	}

	//	fmt.Fprintf(os.Stderr, "event (read %x): %+v\n", i, ev)

	if (ev.Type == 's' || ev.Type == 'f') && w.f&Lspans == 0 {
		return len(p), nil
	}

	b := w.b[:0]

	name, file, line := ev.PC.NameFileLine()

	if w.f&Lmessagespan != 0 || w.f&Lspans != 0 && (ev.Type == 's' || ev.Type == 'f') {
		b = w.spanHeader(b, ev.Span, ev.Level, ev.Time, name, file, line)
	} else {
		b = w.buildHeader(b, ev.Level, ev.Time, name, file, line)
	}

	st := len(b)

	b = append(b, ev.Message...)

	if len(ev.Labels) != 0 {
		b = append(b, "Labels:"...)

		for _, l := range ev.Labels {
			b = append(b, ' ')
			b = append(b, l...)
		}
	}

	if i < len(p) && p[i] == wire.Semantic|wire.UserFields || ev.Type != 0 || ev.Elapsed != 0 || ev.Value != nil {
		b, i = w.structuredFormatter(b, len(b)-st, &ev, p, i)
	}

	if t := w.d.Tag(p, i); t != 0xff {
		return i, fmt.Errorf("tag %2x at %x/%x", t, i, len(p))
	}

	i++

	b.NewLine()

	w.b = b

	_, err = w.w.Write(b)

	return i, err
}

func (w *Console) buildHeader(b []byte, lv Level, tm int64, fname, file string, line int) []byte {
	var col *ColorConfig
	if w.Colorize {
		col = w.ColorConfig
		if col == nil {
			col = &DefaultColorConfig
		}
	}

	if w.f&(Ldate|Ltime|Lmilliseconds|Lmicroseconds) != 0 {
		t := time.Unix(0, tm)

		if w.f&LUTC != 0 {
			t = t.UTC()
		}

		var Y, M, D, h, m, s int
		if w.f&(Ldate|Ltime) != 0 {
			Y, M, D, h, m, s = low.SplitTime(t)
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
				color = col.Debug
			case lv > Fatal:
				color = col.Levels[Fatal]
			default:
				color = col.Levels[lv]
			}
		}

		if color != 0 {
			if lv >= Error {
				b = append(b, colors[ColorBold]...)
			}
			b = append(b, colors[color]...)
		}

		i := len(b)
		b = append(b, low.Spaces[:w.LevelWidth]...)

		switch lv {
		case Info:
			copy(b[i:], "INFO")
		case Warn:
			copy(b[i:], "WARN")
		case Error:
			copy(b[i:], "ERROR")
		case Fatal:
			copy(b[i:], "FATAL")
		default:
			b = low.AppendPrintf(b[:i], "%*x", w.LevelWidth, lv)
		}

		end := len(b)

		if color != 0 {
			b = append(b, colors[0]...)
		}

		if pad := i + w.LevelWidth + 2 - end; pad > 0 {
			b = append(b, low.Spaces[:pad]...)
		}
	}

	if w.f&(Llongfile|Lshortfile) != 0 {
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

			b = append(b, low.Spaces[:w.Shortfile]...)
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
				b = append(b, low.Spaces[:w.Funcname]...)
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

func (w *Console) spanHeader(b []byte, sid ID, lv Level, tm int64, name, file string, line int) []byte {
	b = w.buildHeader(b, lv, tm, name, file, line)

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

// Copy makes config copy.
// Use it instead of assignment since structure contains fields that should not be copied.
func (c *StructuredConfig) Copy() StructuredConfig {
	return StructuredConfig{
		MessageWidth:     c.MessageWidth,
		ValueMaxPadWidth: c.ValueMaxPadWidth,

		PairSeparator: c.PairSeparator,
		KVSeparator:   c.KVSeparator,

		QuoteAnyValue:   c.QuoteAnyValue,
		QuoteEmptyValue: c.QuoteEmptyValue,
	}
}

//nolint:gocognit
func (w *Console) structuredFormatter(b []byte, msgw int, ev *wire.EventHeader, kvs []byte, i int) ([]byte, int) {
	const digits = "0123456789abcdef"

	if w.StructuredConfig == nil {
		w.StructuredConfig = &DefaultStructuredConfig
	}

	c := w.StructuredConfig

	if w.colKey == nil {
		if w.Colorize {
			col := w.ColorConfig
			if col == nil {
				col = &DefaultColorConfig
			}

			w.colKey = colors[col.AttrKey]
			w.colVal = colors[col.AttrValue]
		} else {
			w.colKey = []byte{}
		}
	}

	if msgw != 0 && msgw < c.MessageWidth {
		b = append(b, low.Spaces[:c.MessageWidth-msgw]...)
	}

	var buf [4]byte
	var bufl int
	var sep bool

	if ev.Type != 0 {
		sep = true

		if ev.Type >= 0x20 && ev.Type < 0x80 {
			buf[0] = byte(ev.Type)
			bufl = 1
		} else {
			buf[0] = digits[ev.Type>>4]
			buf[1] = digits[ev.Type&0xf]
			bufl = 2
		}

		b = w.appendPair(b, "T", low.UnsafeBytesToString(buf[:bufl]))
	}

	if ev.Parent != (ID{}) {
		if sep {
			b = append(b, c.PairSeparator...)
		} else {
			sep = true
		}

		b = w.appendPair(b, "parent", ev.Parent)
	}

	if ev.Elapsed != 0 {
		if sep {
			b = append(b, c.PairSeparator...)
		} else {
			sep = true
		}

		b = w.appendPair(b, "elapsed_ms", ev.Elapsed.Seconds()*1000)
	}

	if ev.Value != nil {
		if sep {
			b = append(b, c.PairSeparator...)
		} else {
			sep = true
		}

		b = w.appendPair(b, "value", ev.Value)
	}

	i++ // UserFields

	t := w.d.Tag(kvs, i) & wire.TypeMask
	if t != wire.Map {
		return b, i
	}

	els, i := w.d.NextInt(kvs, i)

	var k, v interface{}
	for el := 0; i < len(kvs) && (els == -1 || el < els); el++ {
		//	fmt.Fprintf(os.Stderr, "parse el %x / %x  i %2x\n", el, els, i)
		if els == -1 && kvs[i] == wire.Spec|wire.Break {
			i++
			break
		}

		if sep {
			b = append(b, c.PairSeparator...)
		} else {
			sep = true
		}

		k, i = w.d.NextValue(kvs, i)
		v, i = w.d.NextValue(kvs, i)

		var ks string
		switch k := k.(type) {
		case string:
			ks = k
		case []byte:
			ks = low.UnsafeBytesToString(k)
		default:
			panic(k)
		}

		b = w.appendPair(b, ks, v)
	}

	return b, i
}

func (w *Console) appendPair(b []byte, k string, v interface{}) []byte {
	c := w.StructuredConfig

	if len(w.colKey) != 0 {
		b = append(b, w.colKey...)
	}

	//	kst := len(b)

	b = append(b, k...)

	b = append(b, c.KVSeparator...)

	//	kend := len(b)

	if len(w.colKey) != 0 {
		b = append(b, colors[0]...)
	}

	if len(w.colVal) != 0 {
		b = append(b, w.colVal...)
	}

	vst := len(b)

	b = w.appendValue(b, v)

	vw := len(b) - vst

	if len(w.colVal) != 0 {
		b = append(b, colors[0]...)
	}

	{ // pad
		var w int
		iw, ok := c.structValWidth.Load(k)
		if ok {
			w = iw.(int)
		}

		if !ok || vw > w {
			if vw > c.ValueMaxPadWidth {
				vw = c.ValueMaxPadWidth
			}

			c.structValWidth.Store(k, vw)
		} else if vw < w {
			b = append(b, low.Spaces[:w-vw]...)
		}
	}

	return b
}

func (w *Console) appendValue(b []byte, v interface{}) []byte {
	c := w.StructuredConfig
	if c == nil {
		c = &DefaultStructuredConfig
	}

	switch v := v.(type) {
	case string:
		if c.QuoteAnyValue || c.QuoteEmptyValue && v == "" || needQuote(v) {
			b = strconv.AppendQuote(b, v)
		} else {
			b = append(b, v...)
		}
	case []byte:
		b = w.appendBytes(b, v)
	case ID:
		i := len(b)
		b = append(b, "123456789_123456789_123456789_12"[:w.IDWidth]...)
		v.FormatTo(b[i:], 'v')
	default:
		b = low.AppendPrintf(b, "%v", v)
	}

	return b
}

func (w *Console) appendBytes(b, v []byte) []byte {
	const digits = "0123456789abcdef"

	for _, c := range v {
		b = append(b, ' ', digits[c>>8], digits[c&0xf])
	}

	return b
}

func needQuote(s string) bool {
	for _, c := range s {
		if c == '"' || c == '`' {
			return true
		}
		if !strconv.IsPrint(c) {
			return true
		}
	}

	return false
}
