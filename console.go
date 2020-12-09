package tlog

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/nikandfor/loc"
	"github.com/nikandfor/tlog/low"
	"golang.org/x/crypto/ssh/terminal"
)

type (
	ConsoleWriter struct {
		io.Writer
		f int

		d Decoder

		b, h low.Buf

		ls Labels

		Colorize        bool
		PadEmptyMessage bool

		LevelWidth   int
		MessageWidth int
		IDWidth      int
		Shortfile    int
		Funcname     int
		MaxValPad    int

		TimeFormat string

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
	Llevel // log level

	LstdFlags = Ldate | Ltime
	LdetFlags = Ldate | Ltime | Lmicroseconds | Lshortfile | Llevel

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
		f:      f,

		Colorize:        colorize,
		PadEmptyMessage: true,

		LevelWidth:   3,
		Shortfile:    18,
		Funcname:     16,
		MessageWidth: 30,
		IDWidth:      8,
		MaxValPad:    24,

		TimeFormat: "2006-01-02_15:04:05.000",

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
	i := 0

	defer func() {
		perr := recover()

		if err == nil && perr == nil {
			return
		}

		if perr != nil {
			fmt.Fprintf(w.Writer, "panic: %v (pos %x)\n", perr, i)
		} else {
			fmt.Fprintf(w.Writer, "parse error: %v (pos %x)\n", err, i)
		}
		fmt.Fprintf(w.Writer, "dump\n%v", Dump(p))

		s := debug.Stack()
		fmt.Fprintf(w.Writer, "%s", s)
	}()

	w.d.Err = nil

	var ts Timestamp
	var pc loc.PC
	var m []byte
	var lv int

	b := w.b

	tag, els, i := w.d.NextTag(p, i)
	if tag != Map {
		return 0, errors.New("expected map")
	}

	var k []byte
	for el := 0; els == -1 || el < els && el < 8; el++ {
		if els == -1 && w.d.NextBreak(p, &i) {
			break
		}

		tag, k, i = w.d.NextString(p, i)
		if w.d.Err != nil {
			return 0, w.d.Err
		}

		if len(k) == 0 {
			return 0, errors.New("empty key")
		}

		ks := low.UnsafeBytesToString(k)
		switch ks {
		case KeyTime:
			ts, i = w.d.NextTime(p, i)
		case KeyMessage:
			_, m, i = w.d.NextString(p, i)
		case KeyLogLevel:
			_, lv, i = w.d.NextTag(p, i)
		case KeyLocation:
			pc, i = w.d.NextLoc(p, i)
		default:
			b, i = w.appendPair(b, k, p, i)
		}
	}

	h := w.h
	h = w.appendHeader(h, ts, lv, pc, m, len(b))

	h = append(h, b...)

	h.NewLine()

	w.b = b[:0]
	w.h = h[:0]

	_, err = w.Writer.Write(h)

	return len(p), err
}

func (w *ConsoleWriter) appendHeader(b []byte, ts Timestamp, lv int, pc loc.PC, m []byte, blen int) []byte {
	var fname, file string
	line := -1

	if w.f&(Ldate|Ltime|Lmilliseconds|Lmicroseconds) != 0 {
		var t time.Time
		if ts != 0 {
			t = time.Unix(0, int64(ts))
		}

		if w.f&LUTC != 0 {
			t = t.UTC()
		}

		var Y, M, D, h, m, s int
		if w.f&(Ldate|Ltime) != 0 {
			Y, M, D, h, m, s = low.SplitTime(t)
		}

		if w.Colorize && len(w.TimeColor) != 0 {
			b = append(b, w.TimeColor...)
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

		if w.Colorize && len(w.TimeColor) != 0 {
			b = append(b, ResetColor...)
		}

		b = append(b, ' ', ' ')
	}

	if w.f&Llevel != 0 {
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

	if w.f&(Llongfile|Lshortfile) != 0 {
		fname, file, line = pc.NameFileLine()

		if w.Colorize && len(w.FileColor) != 0 {
			b = append(b, w.FileColor...)
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

		if w.Colorize && len(w.FileColor) != 0 {
			b = append(b, ResetColor...)
		}

		b = append(b, ' ', ' ')
	}

	if w.f&(Ltypefunc|Lfuncname) != 0 {
		if line == -1 {
			fname, _, _ = pc.NameFileLine()
		}
		fname = filepath.Base(fname)

		if w.Colorize && len(w.FuncColor) != 0 {
			b = append(b, w.FuncColor...)
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

		if w.Colorize && len(w.FuncColor) != 0 {
			b = append(b, ResetColor...)
		}

		b = append(b, ' ', ' ')
	}

	if w.PadEmptyMessage || len(m) != 0 {
		if w.Colorize && len(w.MessageColor) != 0 {
			b = append(b, w.MessageColor...)
		}

		b = append(b, m...)

		if w.Colorize && len(w.MessageColor) != 0 {
			b = append(b, ResetColor...)
		}

		if len(m) < w.MessageWidth && blen != 0 {
			b = append(b, low.Spaces[:w.MessageWidth-len(m)]...)
		}
	}

	return b
}

func (w *ConsoleWriter) appendPair(b, k, p []byte, i int) ([]byte, int) {
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

	st := len(b)

	b, i = w.convertValue(b, p, i)

	vw := len(b) - st

	if w.Colorize && len(w.ValColor) != 0 {
		b = append(b, ResetColor...)
	}

	nw := w.pad[low.UnsafeBytesToString(k)]

	if vw < nw && i+1 < len(p) {
		b = append(b, low.Spaces[:nw-vw]...)
	}

	if nw < vw && nw < w.MaxValPad {
		if vw > w.MaxValPad {
			vw = w.MaxValPad
		}

		w.pad[string(k)] = vw
	}

	return b, i
}

func (w *ConsoleWriter) convertValue(b, p []byte, st int) ([]byte, int) {
	tag, sub, i := w.d.NextTag(p, st)

	switch tag {
	case Int:
		var v int64
		tag, v, i = w.d.NextInt(p, st)

		b = strconv.AppendUint(b, uint64(v), 10)
	case Neg:
		var v int64
		tag, v, i = w.d.NextInt(p, st)

		b = strconv.AppendInt(b, v, 10)
	case Bytes, String:
		var s []byte
		tag, s, i = w.d.NextString(p, st)

		ss := low.UnsafeBytesToString(s)
		if tag == Bytes || w.QuoteAnyValue || strings.ContainsAny(ss, w.QuoteChars) || len(s) == 0 && w.QuoteEmptyValue {
			b = strconv.AppendQuote(b, ss)
		} else {
			b = append(b, s...)
		}
	case Array:
		b = append(b, '[')

		for el := 0; sub == -1 || el < sub; el++ {
			if sub == -1 && w.d.NextBreak(p, &i) {
				break
			}

			if el != 0 {
				b = append(b, ' ')
			}

			b, i = w.convertValue(b, p, i)
		}

		b = append(b, ']')
	case Map:
		b = append(b, '{')

		for el := 0; sub == -1 || el < sub; el++ {
			if sub == -1 && w.d.NextBreak(p, &i) {
				break
			}

			if el != 0 {
				b = append(b, ' ')
			}

			b, i = w.convertValue(b, p, i)

			b = append(b, ':')

			b, i = w.convertValue(b, p, i)
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
		case Float32, Float64:
			var f float64
			f, i = w.d.NextFloat(p, st)

			b = strconv.AppendFloat(b, f, 'f', 5, 64)
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

			ts, i = w.d.NextTime(p, st)

			t := time.Unix(0, int64(ts))
			if w.f&LUTC != 0 {
				t = t.UTC()
			}
			b = t.AppendFormat(b, w.TimeFormat)

			return b, i
		case WireID:
			var id ID
			id, i = w.d.NextID(p, st)

			st := len(b)
			b = append(b, "123456789_123456789_123456789_12"[:w.IDWidth]...)
			id.FormatTo(b[st:], 'v')

			return b, i
		case WireHex:
			tag, sub, _ = w.d.NextTag(p, i)

			switch tag {
			case Int, Neg:
				var v int64
				_, v, i = w.d.NextInt(p, i)

				b = low.AppendPrintf(b, "%x", uint64(v))

				return b, i
			case Bytes, String:
				var s []byte
				_, s, i = w.d.NextString(p, i)

				b = low.AppendPrintf(b, "%x", s)

				return b, i
			}
		}

		b, i = w.convertValue(b, p, i)
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
