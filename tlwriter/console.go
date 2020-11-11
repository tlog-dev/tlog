package tlwriter

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh/terminal"

	"github.com/nikandfor/tlog/low"
	"github.com/nikandfor/tlog/tlt"
	"github.com/nikandfor/tlog/wire"
)

type (
	ID    = tlt.ID
	Type  = tlt.Type
	Level = tlt.Level

	Console struct {
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
		Shortfile:  20,
		Funcname:   18,
		IDWidth:    8,
		LevelWidth: 3,
		Colorize:   colorize,
	}
}

func (w *Console) Write(p []byte) (_ int, err error) {
	var (
		id, par ID
		tp      Type
		lv      Level
		tm      int64
		pc      uint64
		el      time.Duration
		ls      tlt.Labels
		val     interface{}

		msg []byte

		name, file string
		line       int
	)

	//	fmt.Fprintf(os.Stderr, "message\n%v", hex.Dump(p))

	// decode header
	_ = pc
	i := 0
loop:
	for i < len(p) {
		t := p[i]
		i++

		//	fmt.Fprintf(os.Stderr, "parsetag  i %3x  t %2x\n", i-1, t)

		switch t & wire.TypeMask {
		case wire.Map:
			i--
			break loop
		case wire.Semantic:
			// ok
		default:
			panic(t)
		}

		t &^= wire.Semantic

		switch t {
		case wire.EOR:
			i--
			break loop
		case wire.Time: // time
			if p[i]&wire.TypeDetMask != 0x1f {
				panic(p[i])
			}
			i++ // int64
			tm = int64(binary.BigEndian.Uint64(p[i:]))
			i += 8
		case wire.Type: // Type
			//	fmt.Fprintf(os.Stderr, "get type %x %[1]q %x %[2]q\n", p[i], p[i+1])
			i++ // int8
			tp = Type(p[i])
			i++
		case wire.Level: // Level
			lv = Level(p[i] & wire.TypeDetMask)
			if p[i]&wire.TypeMask == wire.Neg {
				lv = -lv
			}
			i++
		case wire.ID: // ID
			l := int(p[i] & 0x1f)
			i++
			i += copy(id[:l], p[i:])
		case wire.Parent:
			l := int(p[i] & 0x1f)
			i++
			i += copy(par[:l], p[i:])
		case wire.Message: // Message/Name
			var l int64
			l, i = getint(p, i)

			msg = p[i : i+int(l)]
			i += int(l)
		case wire.Duration:
			var v int64
			v, i = getint(p, i)

			el = time.Duration(v)
		case wire.Labels:
			l := int(p[i] & 0x1f)
			i++

			var q interface{}
			for j := 0; j < l; j++ {
				q, i = getvalue(p, i)

				ls = append(ls, q.(string))
			}
		case wire.Value:
			val, i = getvalue(p, i)
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

	st := len(b)

	b = append(b, msg...)

	if ls != nil {
		b = append(b, "Labels:"...)

		for _, l := range ls {
			b = append(b, ' ')
			b = append(b, l...)
		}
	}

	if p[i] == wire.Map || tp != 0 || el != 0 || val != nil {
		b, i = w.structuredFormatter(b, len(b)-st, tp, par, el, val, p, i)
	}

	if p[i] != wire.Semantic|wire.EOR {
		panic(p[i])
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

// DefaultStructuredConfig is default config to format structured logs by Console writer.
var DefaultStructuredConfig = StructuredConfig{
	MessageWidth:     40,
	ValueMaxPadWidth: 20,
	PairSeparator:    "  ",
	KVSeparator:      "=",
}

//nolint:gocognit
func (w *Console) structuredFormatter(b []byte, msgw int, tp Type, par ID, el time.Duration, val interface{}, kvs []byte, i int) ([]byte, int) {
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
		b = append(b, low.Spaces[:c.MessageWidth-msgw]...)
	}

	var sep bool

	if tp >= 0x20 && tp < 0x80 {
		sep = true

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

	if par != (ID{}) {
		if sep {
			b = append(b, c.PairSeparator...)
		} else {
			sep = true
		}

		if colKey != nil {
			b = append(b, colKey...)
		}

		b = append(b, "parent"...)

		b = append(b, c.KVSeparator...)

		if colKey != nil {
			b = append(b, colors[0]...)
		}

		if colVal != nil {
			b = append(b, colVal...)
		}

		b = w.appendValue(b, par)

		if colVal != nil {
			b = append(b, colors[0]...)
		}
	}

	if el != 0 {
		if sep {
			b = append(b, c.PairSeparator...)
		} else {
			sep = true
		}

		if colKey != nil {
			b = append(b, colKey...)
		}

		b = append(b, "elapsed_ms"...)

		b = append(b, c.KVSeparator...)

		if colKey != nil {
			b = append(b, colors[0]...)
		}

		if colVal != nil {
			b = append(b, colVal...)
		}

		st := len(b)
		b = append(b, low.Spaces[:10]...)
		b = strconv.AppendFloat(b[:st], el.Seconds()*1000, 'f', 2, 64)
		b = b[:st+10]

		if colVal != nil {
			b = append(b, colors[0]...)
		}
	}

	if val != nil {
		if sep {
			b = append(b, c.PairSeparator...)
		} else {
			sep = true
		}

		if colKey != nil {
			b = append(b, colKey...)
		}

		b = append(b, "value"...)

		b = append(b, c.KVSeparator...)

		if colKey != nil {
			b = append(b, colors[0]...)
		}

		if colVal != nil {
			b = append(b, colVal...)
		}

		switch val.(type) {
		case float64, float32:
			b = low.AppendPrintf(b, "%11.5f", val)
		default:
			b = low.AppendPrintf(b, "%11v", val)
		}

		if colVal != nil {
			b = append(b, colors[0]...)
		}
	}

	t := kvs[i] & wire.TypeMask
	els := int(kvs[i] & wire.TypeDetMask)
	if t != wire.Map {
		return b, i
	}

	i++

	var v interface{}
	for el := 0; i < len(kvs) && (els == 0 || el < els); el++ {
		if els == 0 && kvs[i] == wire.Spec|wire.Break {
			i++
			break
		}

		if sep {
			b = append(b, c.PairSeparator...)
		} else {
			sep = true
		}

		v, i = getvalue(kvs, i)

		if colKey != nil {
			b = append(b, colKey...)
		}

		kst := len(b)

		b = w.appendValue(b, v)

		kend := len(b)

		b = append(b, c.KVSeparator...)

		if colKey != nil {
			b = append(b, colors[0]...)
		}

		v, i = getvalue(kvs, i)

		if colVal != nil {
			b = append(b, colVal...)
		}

		vst := len(b)

		b = w.appendValue(b, v)

		vw := len(b) - vst

		if colVal != nil {
			b = append(b, colors[0]...)
		}

		if vw < c.ValueMaxPadWidth && i+1 < len(kvs) {
			k := low.UnsafeBytesToString(b[kst:kend])

			var w int
			iw, ok := c.structValWidth.Load(k)
			if ok {
				w = iw.(int)
			}

			if !ok || vw > w {
				c.structValWidth.Store(k, vw)
			} else if vw < w {
				b = append(b, low.Spaces[:w-vw]...)
			}
		}
	}

	return b, i
}

func (w *Console) appendValue(b []byte, v interface{}) []byte {
	const escape = `"'`

	c := w.StructuredConfig
	if c == nil {
		c = &DefaultStructuredConfig
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
		b = low.AppendPrintf(b, "%v", v)
	}

	return b
}

func getint(b []byte, i int) (l int64, _ int) {
	t := b[i] & wire.TypeMask

	tl := int64(b[i] & wire.TypeDetMask)
	i++

	switch tl {
	default:
		l = tl
	case 1<<5 - 1:
		l |= int64(b[i]) << 56
		i++
		l |= int64(b[i]) << 48
		i++
		l |= int64(b[i]) << 40
		i++
		l |= int64(b[i]) << 32
		i++

		fallthrough
	case 1<<5 - 2:
		l |= int64(b[i]) << 24
		i++
		l |= int64(b[i]) << 16
		i++

		fallthrough
	case 1<<5 - 3:
		l |= int64(b[i]) << 8
		i++

		fallthrough
	case 1<<5 - 4:
		l |= int64(b[i])
		i++
	}

	if t == wire.Neg {
		l = -l
	}

	return l, i
}

func getvalue(b []byte, i int) (val interface{}, ri int) {
	t := b[i]
	i++

	tl := int(t & wire.TypeDetMask)

	var l int
	switch tl {
	default:
		l = tl
	case 1<<5 - 1:
		l |= int(b[i]) << 56
		i++
		l |= int(b[i]) << 48
		i++
		l |= int(b[i]) << 40
		i++
		l |= int(b[i]) << 32
		i++

		fallthrough
	case 1<<5 - 2:
		l |= int(b[i]) << 24
		i++
		l |= int(b[i]) << 16
		i++

		fallthrough
	case 1<<5 - 3:
		l |= int(b[i]) << 8
		i++

		fallthrough
	case 1<<5 - 4:
		l |= int(b[i])
		i++
	}

	//	defer func() {
	//		fmt.Fprintf(os.Stderr, "getvalue  i %x  t %2x tl %2x l %2x  -> %T %[5]v %x\n", i-1, t, tl, l, val, ri)
	//	}()

	switch t & wire.TypeMask {
	case wire.Int:
		return int64(l), i
	case wire.Neg:
		return -int64(l), i
	case wire.Bytes:
		return b[i : i+l], i + l
	case wire.String:
		return low.UnsafeBytesToString(b[i : i+l]), i + l
	case wire.Spec:
		switch t & wire.TypeDetMask {
		case wire.False:
			return false, i
		case wire.True:
			return true, i
		case wire.Float32:
			f := math.Float32frombits(binary.BigEndian.Uint32(b[i:]))
			return f, i + 4
		case wire.Float64:
			f := math.Float64frombits(binary.BigEndian.Uint64(b[i:]))
			return f, i + 8
		default:
			panic(t)
		}
	default:
		panic(t)
	}
}
