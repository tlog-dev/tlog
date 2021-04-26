package tlog

import (
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/nikandfor/errors"
	"github.com/nikandfor/loc"
	"github.com/nikandfor/tlog/low"
	"golang.org/x/crypto/ssh/terminal"
)

type (
	ConsoleWriter struct {
		io.Writer
		Flags int

		d Decoder

		addpad int     // padding for the next pair
		b, h   low.Buf // buf, header

		ls Labels

		Colorize        bool
		PadEmptyMessage bool

		LevelWidth   int
		MessageWidth int
		IDWidth      int
		Shortfile    int
		Funcname     int
		MaxValPad    int

		TimeFormat     string
		DurationFormat string
		DurationDiv    time.Duration
		FloatFormat    string
		FloatChar      byte
		FloatPrecision int
		LocationFormat string

		PairSeparator string
		KVSeparator   string

		QuoteChars      string
		QuoteAnyValue   bool
		QuoteEmptyValue bool

		TimeColor    []byte
		FileColor    []byte
		FuncColor    []byte
		MessageColor []byte
		KeyColor     []byte
		ValColor     []byte
		LevelColor   struct {
			Info  []byte
			Warn  []byte
			Error []byte
			Fatal []byte
			Debug []byte
		}

		pad map[string]int
	}
)

const ( // console writer flags
	Ldate = 1 << iota
	Ltime
	Lseconds
	Lmilliseconds
	Lmicroseconds
	Lshortfile
	Llongfile
	Ltypefunc // pkg.(*Type).Func
	Lfuncname // Func
	LUTC
	Lloglevel // log level

	LstdFlags = Ldate | Ltime
	LdetFlags = Ldate | Ltime | Lmicroseconds | Lshortfile | Lloglevel

	Lnone = 0
)

var (
	ResetColor = Color(0)
)

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
		Writer: w,
		Flags:  f,

		Colorize:        colorize,
		PadEmptyMessage: true,

		LevelWidth:   3,
		Shortfile:    18,
		Funcname:     16,
		MessageWidth: 30,
		IDWidth:      8,
		MaxValPad:    24,

		TimeFormat:     "2006-01-02_15:04:05.000",
		DurationFormat: "%v",
		FloatChar:      'f',
		FloatPrecision: 5,
		LocationFormat: "%v",

		PairSeparator: "  ",
		KVSeparator:   "=",

		QuoteChars:      "`\"' ()[]{}*",
		QuoteEmptyValue: true,

		TimeColor: Color(90),
		FileColor: Color(90),
		FuncColor: Color(90),
		KeyColor:  Color(36),
		LevelColor: struct {
			Info  []byte
			Warn  []byte
			Error []byte
			Fatal []byte
			Debug []byte
		}{
			Info:  Color(90),
			Warn:  Color(31),
			Error: Color(31, 1),
			Fatal: Color(31, 1),
			Debug: Color(90),
		},

		pad: make(map[string]int),
	}
}

func (w *ConsoleWriter) Write(p []byte) (_ int, err error) {
	i := int64(0)

	defer func() {
		perr := recover()

		if err == nil && perr == nil {
			return
		}

		if perr != nil {
			fmt.Fprintf(w.Writer, "panic: %v (pos %x)\n", perr, i)
		} else {
			fmt.Fprintf(w.Writer, "parse error: %+v (pos %x)\n", err, i)
		}
		fmt.Fprintf(w.Writer, "dump\n%v", Dump(p))
		fmt.Fprintf(w.Writer, "hex dump\n%v", hex.Dump(p))

		s := debug.Stack()
		fmt.Fprintf(w.Writer, "%s", s)
	}()

	w.d.ResetBytes(p)
	w.addpad = 0

	var ts Timestamp
	var pc loc.PC
	var pcs loc.PCs
	var lv LogLevel
	var m []byte
	b := w.b

again:
	tag, els, i := w.d.Tag(i)
	if err = w.d.Err(); err != nil {
		return
	}

	if tag == Semantic && els == WireMeta {
		i = w.d.Skip(i)
		goto again
	}

	if tag != Map {
		return 0, errors.New("expected map")
	}

	var k []byte
	var sub int
	for el := 0; els == -1 || el < els; el++ {
		if els == -1 && w.d.Break(&i) {
			break
		}

		k, i = w.d.String(i)
		if err = w.d.Err(); err != nil {
			return 0, err
		}
		if len(k) == 0 {
			return 0, errors.New("empty key")
		}

		st := i

		tag, sub, i = w.d.Tag(i)
		if tag != Semantic {
			b, i = w.appendPair(b, k, st)
			continue
		}

		ks := low.UnsafeBytesToString(k)
		switch {
		case ks == KeyTime && sub == WireTime:
			ts, i = w.d.Time(st)
		case ks == KeyLocation && sub == WireLocation:
			pc, pcs, i = w.d.Location(st)

			if pcs != nil && len(pcs) != 0 {
				pc = pcs[0]
			}
		case ks == KeyMessage && sub == WireMessage:
			m, i = w.d.String(i)
		case ks == KeyLogLevel && sub == WireLogLevel && w.Flags&Lloglevel != 0:
			lv, i = w.d.LogLevel(st)
		default:
			b, i = w.appendPair(b, k, st)
		}
	}

	if err = w.d.Err(); err != nil {
		return
	}

	h := w.h
	h = w.appendHeader(w.h, ts, lv, pc, m, len(b))

	h = append(h, b...)

	h.NewLine()

	w.b = b[:0]
	w.h = h[:0]

	_, err = w.Writer.Write(h)

	return len(p), err
}

func (w *ConsoleWriter) appendHeader(b []byte, ts Timestamp, lv LogLevel, pc loc.PC, m []byte, blen int) []byte {
	var fname, file string
	line := -1

	if w.Flags&(Ldate|Ltime|Lmilliseconds|Lmicroseconds) != 0 {
		var t time.Time
		if ts != 0 {
			t = time.Unix(0, int64(ts))
		}

		if w.Flags&LUTC != 0 {
			t = t.UTC()
		}

		var Y, M, D, h, m, s int
		if w.Flags&(Ldate|Ltime) != 0 {
			Y, M, D, h, m, s = low.SplitTime(t)
		}

		if w.Colorize && len(w.TimeColor) != 0 {
			b = append(b, w.TimeColor...)
		}

		if w.Flags&Ldate != 0 {
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
		if w.Flags&Ltime != 0 {
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
		if w.Flags&(Lmilliseconds|Lmicroseconds) != 0 {
			if len(b) != 0 {
				b = append(b, '.')
			}

			ns := t.Nanosecond() / 1e3
			if w.Flags&Lmilliseconds != 0 {
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

		if w.Colorize && len(w.TimeColor) != 0 {
			b = append(b, ResetColor...)
		}

		b = append(b, ' ', ' ')
	}

	if w.Flags&Lloglevel != 0 {
		var col []byte
		switch {
		case !w.Colorize:
			// break
		case lv == Info:
			col = w.LevelColor.Info
		case lv == Warn:
			col = w.LevelColor.Warn
		case lv == Error:
			col = w.LevelColor.Error
		case lv >= Fatal:
			col = w.LevelColor.Fatal
		default:
			col = w.LevelColor.Debug
		}

		if col != nil {
			b = append(b, col...)
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

		if col != nil {
			b = append(b, ResetColor...)
		}

		if pad := i + w.LevelWidth + 2 - end; pad > 0 {
			b = append(b, low.Spaces[:pad]...)
		}
	}

	if w.Flags&(Llongfile|Lshortfile) != 0 {
		fname, file, line = pc.NameFileLine()

		if w.Colorize && len(w.FileColor) != 0 {
			b = append(b, w.FileColor...)
		}

		if w.Flags&Lshortfile != 0 {
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

			n := 1
			for q := line; q != 0; q /= 10 {
				n++
			}

			i := len(b)
			b = append(b, ":           "[:n]...)

			for q, j := line, n-1; j >= 1; j-- {
				b[i+j] = byte(q%10) + '0'
				q /= 10
			}
		}

		if w.Colorize && len(w.FileColor) != 0 {
			b = append(b, ResetColor...)
		}

		b = append(b, ' ', ' ')
	}

	if w.Flags&(Ltypefunc|Lfuncname) != 0 {
		if line == -1 {
			fname, _, _ = pc.NameFileLine()
		}
		fname = filepath.Base(fname)

		if w.Colorize && len(w.FuncColor) != 0 {
			b = append(b, w.FuncColor...)
		}

		if w.Flags&Lfuncname != 0 {
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

		if w.Colorize && len(w.FuncColor) != 0 {
			b = append(b, ResetColor...)
		}

		b = append(b, ' ', ' ')
	}

	if len(m) != 0 {
		if w.Colorize && len(w.MessageColor) != 0 {
			b = append(b, w.MessageColor...)
		}

		b = append(b, m...)

		if w.Colorize && len(w.MessageColor) != 0 {
			b = append(b, ResetColor...)
		}
	}

	if len(m) >= w.MessageWidth && blen != 0 {
		b = append(b, ' ', ' ')
	}

	if (w.PadEmptyMessage || len(m) != 0) && len(m) < w.MessageWidth && blen != 0 {
		b = append(b, low.Spaces[:w.MessageWidth-len(m)]...)
	}

	return b
}

func (w *ConsoleWriter) appendPair(b []byte, k []byte, st int64) (_ []byte, i int64) {
	i = st

	if w.addpad != 0 {
		b = append(b, low.Spaces[:w.addpad]...)
		w.addpad = 0
	}

	if len(b) != 0 {
		b = append(b, w.PairSeparator...)
	}

	if w.Colorize && len(w.KeyColor) != 0 {
		b = append(b, w.KeyColor...)
	}

	b = append(b, k...)

	b = append(b, w.KVSeparator...)

	if w.Colorize && len(w.ValColor) != 0 {
		b = append(b, w.ValColor...)
	} else if w.Colorize && len(w.KeyColor) != 0 {
		b = append(b, ResetColor...)
	}

	vst := len(b)

	b, i = w.convertValue(b, i)

	vw := len(b) - vst

	if w.Colorize && len(w.ValColor) != 0 {
		b = append(b, ResetColor...)
	}

	nw := w.pad[low.UnsafeBytesToString(k)]

	if vw < nw {
		w.addpad = nw - vw
	}

	if nw < vw && nw < w.MaxValPad {
		if vw > w.MaxValPad {
			vw = w.MaxValPad
		}

		w.pad[string(k)] = vw
	}

	return b, i
}

func (w *ConsoleWriter) convertValue(b []byte, st int64) (_ []byte, i int64) {
	tag, sub, i := w.d.Tag(st)

	switch tag {
	case Int:
		var v int64
		v, i = w.d.Int(st)

		b = strconv.AppendUint(b, uint64(v), 10)
	case Neg:
		var v int64
		v, i = w.d.Int(st)

		b = strconv.AppendInt(b, v, 10)
	case Bytes, String:
		var s []byte
		s, i = w.d.String(st)

		quote := tag == Bytes || w.QuoteAnyValue || len(s) == 0 && w.QuoteEmptyValue
		if !quote {
			for _, c := range s {
				if c < 0x20 || c >= 0x80 {
					quote = true
					break
				}
				for _, q := range w.QuoteChars {
					if byte(q) == c {
						quote = true
						break
					}
				}
			}
		}

		if quote {
			ss := low.UnsafeBytesToString(s)
			b = strconv.AppendQuote(b, ss)
		} else {
			b = append(b, s...)
		}
	case Array:
		b = append(b, '[')

		for el := 0; sub == -1 || el < sub; el++ {
			if sub == -1 && w.d.Break(&i) {
				break
			}

			if el != 0 {
				b = append(b, ' ')
			}

			b, i = w.convertValue(b, i)
		}

		b = append(b, ']')
	case Map:
		b = append(b, '{')

		for el := 0; sub == -1 || el < sub; el++ {
			if sub == -1 && w.d.Break(&i) {
				break
			}

			if el != 0 {
				b = append(b, ' ')
			}

			b, i = w.convertValue(b, i)

			b = append(b, ':')

			b, i = w.convertValue(b, i)
		}

		b = append(b, '}')
	case Special:
		switch sub {
		case False:
			b = append(b, "false"...)
		case True:
			b = append(b, "true"...)
		case Null:
			b = append(b, "<nil>"...)
		case Undefined:
			b = append(b, "<undef>"...)
		case Float64, Float32, FloatInt8:
			var f float64
			f, i = w.d.Float(st)

			if w.FloatFormat != "" {
				b = low.AppendPrintf(b, w.FloatFormat, f)
			} else {
				b = strconv.AppendFloat(b, f, w.FloatChar, w.FloatPrecision, 64)
			}
		default:
			panic(sub)
		}
	case Semantic:
		switch sub {
		case WireTime:
			if w.TimeFormat == "" {
				break
			}

			var ts Timestamp
			ts, i = w.d.Time(st)

			t := time.Unix(0, int64(ts))
			if w.Flags&LUTC != 0 {
				t = t.UTC()
			}
			b = t.AppendFormat(b, w.TimeFormat)
		case WireDuration:
			var v int64
			v, i = w.d.Int(i)

			switch {
			case w.DurationFormat != "" && w.DurationDiv != 0:
				b = low.AppendPrintf(b, w.DurationFormat, float64(time.Duration(v)/w.DurationDiv))
			case w.DurationFormat != "":
				b = low.AppendPrintf(b, w.DurationFormat, time.Duration(v))
			default:
				b = strconv.AppendInt(b, v, 10)
			}
		case WireID:
			var id ID
			id, i = w.d.ID(st)

			st := len(b)
			b = append(b, "123456789_123456789_123456789_12"[:w.IDWidth]...)
			id.FormatTo(b[st:], 'v')
		case WireHex:
			tag, sub, _ = w.d.Tag(i)

			switch tag {
			case Int, Neg:
				var v int64
				v, i = w.d.Int(i)

				if tag == Neg {
					b = append(b, "-0x"...)
				} else {
					b = append(b, "0x"...)
				}

				if v < 0 {
					v = -v
				}

				b = strconv.AppendUint(b, uint64(v), 16)
			case Bytes, String:
				var s []byte
				s, i = w.d.String(i)

				b = low.AppendPrintf(b, "%x", s)
			default:
				b, i = w.convertValue(b, i)
			}
		case WireLocation:
			var pc loc.PC
			var pcs loc.PCs

			pc, pcs, i = w.d.Location(st)

			if pcs == nil {
				b = low.AppendPrintf(b, w.LocationFormat, pc)
			} else {
				b = append(b, '[')
				for i, pc := range pcs {
					if i != 0 {
						b = append(b, ',', ' ')
					}

					b = low.AppendPrintf(b, w.LocationFormat, pc)
				}
				b = append(b, ']')
			}
		default:
			b, i = w.convertValue(b, i)
		}
	default:
		panic(tag)
	}

	return b, i
}

func Color(c ...int) (r []byte) {
	if len(c) == 0 {
		return nil
	}

	r = append(r, '\x1b', '[')

	for i, c := range c {
		if i != 0 {
			r = append(r, ';')
		}

		switch {
		case c < 10:
			r = append(r, '0'+byte(c%10))
		case c < 100:
			r = append(r, '0'+byte(c/10), '0'+byte(c%10))
		default:
			r = append(r, '0'+byte(c/100), '0'+byte(c/10%10), '0'+byte(c%10))
		}
	}

	r = append(r, 'm')

	return r
}
