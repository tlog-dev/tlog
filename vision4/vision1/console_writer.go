package tlog

import (
	"bytes"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh/terminal"
)

type (
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

	bufWriter []byte
)

// ConsoleWriter flags. Similar to log.Logger flags.
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
func NewConsoleWriter(w io.Writer, f int) *ConsoleWriter {
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

func (w *ConsoleWriter) Write(ev *Event) (err error) {
	b := ev.b
	st := len(b)

	if w.f&Lmessagespan != 0 || w.f&Lspans != 0 && (ev.Type == 's' || ev.Type == 'f') {
		b = w.spanHeader(b, ev.Span, ev.Level, ev.Time, ev.PC)
	} else {
		b = w.buildHeader(b, ev.Level, ev.Time, ev.PC)
	}

	var txt string
	var txtok bool
	var par ID
	var parok bool

	for i := len(ev.Attrs) - 1; i >= 0; i-- {
		switch ev.Attrs[i].Name {
		case "m":
			if !txtok {
				txt, txtok = ev.Attrs[i].Value.(string)
			}
		case "p":
			if !parok {
				par, parok = ev.Attrs[i].Value.(ID)
			}
		}
	}

	var msglen int
	if txt != "" {
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

		msglen = len(txt)
		b = append(b, txt...)

		if color != 0 {
			b = append(b, colors[0]...)
		}
	}

	if msglen == 0 {
		i := len(b)
		switch ev.Type {
		case 's':
			if par == (ID{}) {
				b = append(b, "Span started"...)
			} else {
				b = append(b, "Span spawned"...)
			}
		case 'f':
			b = append(b, "Span finished"...)
		}
		msglen = len(b) - i
	}

	if len(ev.Attrs) != 0 {
		b = structuredFormatter(w, b, msglen, ev.Type, ev.Attrs)
	}

	b.NewLine()

	_, err = w.w.Write(b[st:])

	return
}

func (w *ConsoleWriter) buildHeader(b []byte, lv Level, t time.Time, loc PC) []byte {
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
		b = append(b, spaces[:w.LevelWidth]...)

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
			b = AppendPrintf(b[:i], "%*x", w.LevelWidth, lv)
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

func (w *ConsoleWriter) spanHeader(b []byte, sid ID, lv Level, tm time.Time, loc PC) []byte {
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

// DefaultStructuredConfig is default config to format structured logs by ConsoleWriter.
var DefaultStructuredConfig = StructuredConfig{
	MessageWidth:     40,
	ValueMaxPadWidth: 20,
	PairSeparator:    "  ",
	KVSeparator:      "=",
}

//nolint:gocognit
func structuredFormatter(w *ConsoleWriter, b []byte, msgw int, tp Type, kvs []A) []byte {
	const escape = `"'`

	c := w.StructuredConfig
	if c == nil {
		c = &DefaultStructuredConfig
	}

	var colKey, colVal []byte
	if w.Colorize {
		col := w.ColorConfig
		if col == nil {
			col = &DefaultColorConfig
		}

		colKey = colors[col.AttrKey]
		colVal = colors[col.AttrValue]
	}

	if msgw != 0 && msgw < c.MessageWidth {
		b = append(b, spaces[:c.MessageWidth-msgw]...)
	}

	if tp >= 0x20 && tp < 0x80 {
		if colKey != nil {
			b = append(b, colKey...)
		}

		b = append(b, 'T')

		b = append(b, c.KVSeparator...)

		if colKey != nil {
			b = append(b, colors[0]...)
		}

		if colVal != nil {
			b = append(b, colVal...)
		}

		b = append(b, byte(tp))

		if colVal != nil {
			b = append(b, colors[0]...)
		}
	}

	for i, kv := range kvs {
		if kv.Name == "m" && msgw != 0 {
			msgw = 0
			continue
		}

		if i != 0 || tp >= 0x20 && tp < 0x80 {
			b = append(b, c.PairSeparator...)
		}

		if colKey != nil {
			b = append(b, colKey...)
		}

		b = append(b, kv.Name...)

		b = append(b, c.KVSeparator...)

		if colKey != nil {
			b = append(b, colors[0]...)
		}

		vst := len(b)

		if colVal != nil {
			b = append(b, colVal...)
		}

		switch v := kv.Value.(type) {
		case string:
			if c.QuoteAnyValue || c.QuoteEmptyValue && v == "" || strings.Contains(v, c.KVSeparator) || strings.ContainsAny(v, escape) {
				b = strconv.AppendQuote(b, v)
			} else {
				b = append(b, v...)
			}
		case []byte:
			if c.QuoteAnyValue || c.QuoteEmptyValue && len(v) == 0 || bytes.Contains(v, []byte(c.KVSeparator)) || bytes.ContainsAny(v, escape) {
				b = strconv.AppendQuote(b, string(v))
			} else {
				b = append(b, v...)
			}
		case ID:
			i := len(b)
			b = append(b, "123456789_123456789_123456789_12"[:w.IDWidth]...)
			v.FormatTo(b[i:], 'v')
		default:
			b = AppendPrintf(b, "%v", kv.Value)
		}

		if colVal != nil {
			b = append(b, colors[0]...)
		}

		vend := len(b)

		vw := vend - vst
		if vw < c.ValueMaxPadWidth && i+1 < len(kvs) {
			var w int
			iw, ok := c.structValWidth.Load(kv.Name)
			if ok {
				w = iw.(int)
			}

			if !ok || vw > w {
				c.structValWidth.Store(kv.Name, vw)
			} else if vw < w {
				b = append(b, spaces[:w-vw]...)
			}
		}
	}

	return b
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
