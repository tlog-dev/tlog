package tlog

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/nikandfor/errors"
	"github.com/nikandfor/hacked/hfmt"
	"github.com/nikandfor/hacked/htime"
	"github.com/nikandfor/loc"
	"github.com/nikandfor/tlog/low"
	"github.com/nikandfor/tlog/tlwire"
	"golang.org/x/term"
)

type (
	ConsoleWriter struct {
		io.Writer
		Flags int

		d tlwire.Decoder

		addpad int     // padding for the next pair
		b, h   low.Buf // buf, header

		ls, lastls []byte

		Colorize        bool
		PadEmptyMessage bool
		AllLabels       bool
		AllCallers      bool

		LevelWidth   int
		MessageWidth int
		IDWidth      int
		Shortfile    int
		Funcname     int
		MaxValPad    int

		TimeFormat     string
		TimeLocation   *time.Location
		DurationFormat string
		DurationDiv    time.Duration
		FloatFormat    string
		FloatChar      byte
		FloatPrecision int
		CallerFormat   string
		BytesFormat    string

		StringOnNewLineMinLen int

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

const (
	cfHex = 1 << iota
)

var (
	ResetColor = Color(0)
)

func NewConsoleWriter(w io.Writer, f int) *ConsoleWriter {
	fd := -1

	switch f := w.(type) {
	case interface {
		Fd() uintptr
	}:
		fd = int(f.Fd())
	case interface {
		Fd() int
	}:
		fd = f.Fd()
	}

	colorize := term.IsTerminal(fd)

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

		TimeFormat:     "2006-01-02_15:04:05.000Z0700",
		DurationFormat: "%v",
		FloatChar:      'f',
		FloatPrecision: 5,
		CallerFormat:   "%v",

		StringOnNewLineMinLen: 71,

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

func (w *ConsoleWriter) Write(p []byte) (i int, err error) {
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
		fmt.Fprintf(w.Writer, "dump\n%v", tlwire.Dump(p))
		fmt.Fprintf(w.Writer, "hex dump\n%v", hex.Dump(p))

		s := debug.Stack()
		fmt.Fprintf(w.Writer, "%s", s)
	}()

	if w.PairSeparator == "" {
		w.PairSeparator = "  "
	}

	if w.KVSeparator == "" {
		w.KVSeparator = "="
	}

	w.addpad = 0

	var t time.Time
	var pc loc.PC
	var lv LogLevel
	var tp EventKind
	var m []byte
	w.ls = w.ls[:0]
	b := w.b

	tag, els, i := w.d.Tag(p, i)
	if tag != tlwire.Map {
		return 0, errors.New("expected map")
	}

	var k []byte
	var sub int64
	for el := 0; els == -1 || el < int(els); el++ {
		if els == -1 && w.d.Break(p, &i) {
			break
		}

		pairst := i

		k, i = w.d.Bytes(p, i)
		if len(k) == 0 {
			return 0, errors.New("empty key")
		}

		st := i

		tag, sub, i = w.d.Tag(p, i)
		if tag != tlwire.Semantic {
			b, i = w.appendPair(b, p, k, st)
			continue
		}

		//	println(fmt.Sprintf("key %s  tag %x %x", k, tag, sub))

		switch {
		case sub == tlwire.Time && string(k) == KeyTimestamp:
			t, i = w.d.Time(p, st)
		case sub == tlwire.Caller && string(k) == KeyCaller:
			var pcs loc.PCs

			pc, pcs, i = w.d.Callers(p, st)

			if w.AllCallers && pcs != nil {
				b, i = w.appendPair(b, p, k, st)
			}
		case sub == WireMessage && string(k) == KeyMessage:
			m, i = w.d.Bytes(p, i)
		case sub == WireLogLevel && string(k) == KeyLogLevel && w.Flags&Lloglevel != 0:
			i = lv.TlogParse(p, st)
		case sub == WireEventKind && string(k) == KeyEventKind:
			_ = tp.TlogParse(p, st)

			b, i = w.appendPair(b, p, k, st)
		case sub == WireLabel:
			i = w.d.Skip(p, st)
			w.ls = append(w.ls, p[pairst:i]...)
		default:
			b, i = w.appendPair(b, p, k, st)
		}
	}

	h := w.h
	h = w.appendHeader(w.h, t, lv, pc, m, len(b))

	h = append(h, b...)

	if w.AllLabels || !bytes.Equal(w.lastls, w.ls) {
		h = w.convertLabels(h, w.ls)
		w.lastls = append(w.lastls[:0], w.ls...)
	}

	h.NewLine()

	w.b = b[:0]
	w.h = h[:0]

	_, err = w.Writer.Write(h)

	return len(p), err
}

func (w *ConsoleWriter) convertLabels(b, p []byte) []byte {
	var k []byte
	i := 0

	for i != len(p) {
		k, i = w.d.Bytes(p, i)
		if len(k) == 0 {
			panic("empty key")
		}

		b, i = w.appendPair(b, p, k, i)
	}

	return b
}

func (w *ConsoleWriter) appendHeader(b []byte, t time.Time, lv LogLevel, pc loc.PC, m []byte, blen int) []byte {
	var fname, file string
	line := -1

	if w.Flags&(Ldate|Ltime|Lmilliseconds|Lmicroseconds) != 0 {
		if w.Flags&LUTC != 0 {
			t = t.UTC()
		}

		var Y, M, D, h, m, s int
		if w.Flags&(Ldate|Ltime) != 0 {
			Y, M, D, h, m, s = htime.DateClock(t)
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
			if w.Flags&Ldate != 0 {
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
			b = hfmt.Appendf(b[:i], "%*x", w.LevelWidth, lv)
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

func (w *ConsoleWriter) appendPair(b, p, k []byte, st int) (_ []byte, i int) {
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

	b, i = w.convertValue(b, p, i, 0)

	vw := len(b) - vst

	// NOTE: Value width can be incorrect for non-ascii symbols.
	// We can calc it by iterating utf8.DecodeRune() but should we?

	if w.Colorize && len(w.ValColor) != 0 {
		b = append(b, ResetColor...)
	}

	nw := w.pad[low.UnsafeBytesToString(k)]

	if vw < nw {
		w.addpad = nw - vw
	}

	if nw < vw && vw <= w.MaxValPad {
		if vw > w.MaxValPad {
			vw = w.MaxValPad
		}

		w.pad[string(k)] = vw
	}

	return b, i
}

func (w *ConsoleWriter) convertValue(b, p []byte, st int, ff int) (_ []byte, i int) {
	tag, sub, i := w.d.Tag(p, st)

	switch tag {
	case tlwire.Int, tlwire.Neg:
		var v uint64
		v, i = w.d.Unsigned(p, st)

		base := 10
		if tag == tlwire.Neg {
			b = append(b, '-')
		}

		if ff&cfHex != 0 {
			b = append(b, "0x"...)
			base = 16
		}

		b = strconv.AppendUint(b, v, base)
	case tlwire.Bytes, tlwire.String:
		var s []byte
		s, i = w.d.Bytes(p, st)

		if w.StringOnNewLineMinLen != 0 && len(s) >= w.StringOnNewLineMinLen {
			b = append(b, '\\', '\n')

			if tag == tlwire.Bytes {
				h := hex.Dumper(noescapeByteWriter(&b))

				_, _ = h.Write(s)
				_ = h.Close()
			} else {
				b = append(b, s...)

				if s[len(s)-1] != '\n' {
					b = append(b, '\n')
				}
			}

			break
		}

		if tag == tlwire.Bytes {
			if w.BytesFormat != "" {
				b = hfmt.Appendf(b, w.BytesFormat, s)
				break
			}

			if ff&cfHex != 0 {
				b = hfmt.Appendf(b, "%x", s)
				break
			}
		}

		quote := tag == tlwire.Bytes || w.QuoteAnyValue || len(s) == 0 && w.QuoteEmptyValue
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
	case tlwire.Array:
		b = append(b, '[')

		for el := 0; sub == -1 || el < int(sub); el++ {
			if sub == -1 && w.d.Break(p, &i) {
				break
			}

			if el != 0 {
				b = append(b, ' ')
			}

			b, i = w.convertValue(b, p, i, ff)
		}

		b = append(b, ']')
	case tlwire.Map:
		b = append(b, '{')

		for el := 0; sub == -1 || el < int(sub); el++ {
			if sub == -1 && w.d.Break(p, &i) {
				break
			}

			if el != 0 {
				b = append(b, ' ')
			}

			b, i = w.convertValue(b, p, i, ff)

			b = append(b, ':')

			b, i = w.convertValue(b, p, i, ff)
		}

		b = append(b, '}')
	case tlwire.Special:
		switch sub {
		case tlwire.None:
			// none
		case tlwire.False:
			b = append(b, "false"...)
		case tlwire.True:
			b = append(b, "true"...)
		case tlwire.Nil:
			b = append(b, "<nil>"...)
		case tlwire.Undefined:
			b = append(b, "<undef>"...)
		case tlwire.Float64, tlwire.Float32, tlwire.Float8:
			var f float64
			f, i = w.d.Float(p, st)

			if w.FloatFormat != "" {
				b = hfmt.Appendf(b, w.FloatFormat, f)
			} else {
				b = strconv.AppendFloat(b, f, w.FloatChar, w.FloatPrecision, 64)
			}
		default:
			panic(sub)
		}
	case tlwire.Semantic:
		switch sub {
		case tlwire.Time:
			var t time.Time
			t, i = w.d.Time(p, st)

			if w.TimeFormat == "" {
				b = strconv.AppendInt(b, t.UnixNano(), 10)
				break
			}

			if w.TimeLocation != nil {
				t = t.In(w.TimeLocation)
			}

			b = t.AppendFormat(b, w.TimeFormat)
		case tlwire.Duration:
			var v int64
			v, i = w.d.Signed(p, i)

			switch {
			case w.DurationFormat != "" && w.DurationDiv != 0:
				b = hfmt.Appendf(b, w.DurationFormat, float64(time.Duration(v)/w.DurationDiv))
			case w.DurationFormat != "":
				b = hfmt.Appendf(b, w.DurationFormat, time.Duration(v))
			default:
				b = strconv.AppendInt(b, v, 10)
			}
		case WireID:
			var id ID
			i = id.TlogParse(p, st)

			st := len(b)
			b = append(b, "123456789_123456789_123456789_12"[:w.IDWidth]...)
			id.FormatTo(b[st:], 'v')
		case tlwire.Hex:
			b, i = w.convertValue(b, p, i, ff|cfHex)
		case tlwire.Caller:
			var pc loc.PC
			var pcs loc.PCs

			pc, pcs, i = w.d.Callers(p, st)

			if pcs == nil {
				b = hfmt.Appendf(b, w.CallerFormat, pc)
				break
			}

			b = append(b, '[')
			for i, pc := range pcs {
				if i != 0 {
					b = append(b, ' ')
				}

				b = hfmt.Appendf(b, w.CallerFormat, pc)
			}
			b = append(b, ']')
		default:
			b, i = w.convertValue(b, p, i, ff)
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

func noescapeByteWriter(b *[]byte) *low.Buf {
	//	return (*low.Buf)(b)
	return (*low.Buf)(noescape(unsafe.Pointer(b)))
}
