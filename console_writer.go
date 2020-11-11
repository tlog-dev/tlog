package tlog

import (
	"bytes"
	"encoding/binary"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

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

		b bufWriter
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

	bwr struct {
		b   bufWriter
		buf [128 - unsafe.Sizeof([]byte{})]byte
	}
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

var spaces = []byte("                                                                                                                                                ")

var bufPool = sync.Pool{New: func() interface{} { w := &bwr{}; w.b = w.buf[:]; return w }}

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

func (w *ConsoleWriter) Write(p []byte) (_ int, err error) {
	var (
		id ID
		tp Type
		lv Level
		tm int64
		pc int32

		msg []byte

		name, file string
		line       int
	)

	// decode header
	_ = pc
	i := 0
	for i < len(p) {
		t := p[i]
		i++

		switch {
		case t == 0:
			break
		case t == 0x01: // time
			tm = int64(binary.BigEndian.Uint64(p[i:]))
			i += 8
		case t == 0x02: // Type
			tp = Type(p[i])
			i++
		case t&0xf0 == 0x01: // Level
			lv = Level(t & 0xf)
			lv = lv << 4 >> 4
			i++
		case t&0xf0 == 0x20: // ID
			l := int(t&0xf) + 1
			i += copy(id[:l], p[i:])
		case t&0xc0 == 0x80: // Message/Name
			l := int(t & 0x3f)
			if l == 0x3f {
				var ll int
				for p[i]&0x80 == 0x80 {
					ll = ll<<8 | int(p[i])&0x7f
					i++
				}

				l += ll<<8 | int(p[i])
				i++
			}

			msg = p[i : i+l]
			i += l
		case t&0xc0 == 0xc0: // Field
			l := int(t & 0x3f)
			if l == 0x3f {
				var ll int
				for p[i]&0x80 == 0x80 {
					ll = ll<<8 | int(p[i])&0x7f
					i++
				}

				l += ll<<8 | int(p[i])
				i++
			}

			i += l
		default:
			panic(t)
		}
	}

	b := w.b[:0]

	if w.f&Lmessagespan != 0 || w.f&Lspans != 0 && (tp == 's' || tp == 'f') {
		b = w.spanHeader(b, id, lv, tm, name, file, line)
	} else {
		b = w.buildHeader(b, lv, tm, name, file, line)
	}

	b = append(b, msg...)

	b.NewLine()

	_, err = w.w.Write(b)
	w.b = b

	return len(p), err
}

func (w *ConsoleWriter) buildHeader(b []byte, lv Level, tm int64, fname, file string, line int) []byte {
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

func (w *ConsoleWriter) spanHeader(b []byte, sid ID, lv Level, tm int64, name, file string, line int) []byte {
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

// DefaultStructuredConfig is default config to format structured logs by ConsoleWriter.
var DefaultStructuredConfig = StructuredConfig{
	MessageWidth:     40,
	ValueMaxPadWidth: 20,
	PairSeparator:    "  ",
	KVSeparator:      "=",
}

//nolint:gocognit
func structuredFormatter(w *ConsoleWriter, b []byte, msgw int, tp Type, kvs []interface{}) []byte {
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

	for i := 0; i < len(kvs); i += 2 {
		k := kvs[i].(string)
		v := kvs[i+1]

		if i != 0 || tp >= 0x20 && tp < 0x80 {
			b = append(b, c.PairSeparator...)
		}

		if colKey != nil {
			b = append(b, colKey...)
		}

		b = append(b, k...)

		b = append(b, c.KVSeparator...)

		if colKey != nil {
			b = append(b, colors[0]...)
		}

		vst := len(b)

		if colVal != nil {
			b = append(b, colVal...)
		}

		switch v := v.(type) {
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
			b = AppendPrintf(b, "%v", v)
		}

		if colVal != nil {
			b = append(b, colors[0]...)
		}

		vend := len(b)

		vw := vend - vst
		if vw < c.ValueMaxPadWidth && i+1 < len(kvs) {
			var w int
			iw, ok := c.structValWidth.Load(k)
			if ok {
				w = iw.(int)
			}

			if !ok || vw > w {
				c.structValWidth.Store(k, vw)
			} else if vw < w {
				b = append(b, spaces[:w-vw]...)
			}
		}
	}

	return b
}

// Getbuf gets bytes buffer from a pool to reduce gc pressure.
// buffer is at least 100 bytes long.
// Buffer must be returned after used. Usage:
//     b, wr := tlog.Getbuf()
//     defer wr.Ret(&b)
//
//     b = append(b[:0], ...)
func Getbuf() (_ bufWriter, wr *bwr) { //nolint:golint
	wr = bufPool.Get().(*bwr)
	return wr.b, wr
}

func (wr *bwr) Ret(b *bufWriter) {
	wr.b = *b
	bufPool.Put(wr)
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
