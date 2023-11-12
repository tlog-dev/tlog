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

	"github.com/nikandfor/hacked/hfmt"
	"github.com/nikandfor/hacked/htime"
	"github.com/nikandfor/hacked/low"
	"golang.org/x/term"
	"tlog.app/go/errors"
	"tlog.app/go/loc"

	tlow "tlog.app/go/tlog/low"
	"tlog.app/go/tlog/tlwire"
)

type (
	ConsoleWriter struct { //nolint:maligned
		io.Writer
		Flags int

		d tlwire.Decoder

		addpad   int     // padding for the next pair
		b, h     low.Buf // buf, header
		lasttime low.Buf

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

		ColorScheme

		pad map[string]int
	}

	ColorScheme struct {
		TimeColor       []byte
		TimeChangeColor []byte // if different from TimeColor
		FileColor       []byte
		FuncColor       []byte
		MessageColor    []byte
		KeyColor        []byte
		ValColor        []byte
		LevelColor      struct {
			Info  []byte
			Warn  []byte
			Error []byte
			Fatal []byte
			Debug []byte
		}
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

	DefaultColorScheme = ColorScheme{
		TimeColor:       Color(90),
		TimeChangeColor: Color(38, 5, 244, 1),
		FileColor:       Color(90),
		FuncColor:       Color(90),
		KeyColor:        Color(36),
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
	}
)

func NewConsoleWriter(w io.Writer, f int) *ConsoleWriter {
	fd := -1

	switch f := w.(type) {
	case interface{ Fd() uintptr }:
		fd = int(f.Fd())
	case interface{ Fd() int }:
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

		TimeFormat: "2006-01-02_15:04:05.000Z0700",
		//DurationFormat: "%v",
		FloatChar:      'f',
		FloatPrecision: 5,
		CallerFormat:   "%v",

		StringOnNewLineMinLen: 71,

		PairSeparator: "  ",
		KVSeparator:   "=",

		QuoteChars:      "`\"' ()[]{}*",
		QuoteEmptyValue: true,

		ColorScheme: DefaultColorScheme,

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

	h := w.h

more:
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

	h = w.appendHeader(h, t, lv, pc, m, len(b))

	h = append(h, b...)

	if w.AllLabels || !bytes.Equal(w.lastls, w.ls) {
		h = w.convertLabels(h, w.ls)
		w.lastls = append(w.lastls[:0], w.ls...)
	}

	h.NewLine()

	if i < len(p) {
		goto more
	}

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
		b = w.appendTime(b, t)

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

func (w *ConsoleWriter) appendTime(b []byte, t time.Time) []byte {
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

	ts := len(b)

	if w.Flags&Ldate != 0 {
		b = append(b,
			byte(Y/1000)+'0',
			byte(Y/100%10)+'0',
			byte(Y/10%10)+'0',
			byte(Y/1%10)+'0',
			'-',
			byte(M/10)+'0',
			byte(M%10)+'0',
			'-',
			byte(D/10)+'0',
			byte(D%10)+'0',
		)
	}
	if w.Flags&Ltime != 0 {
		if w.Flags&Ldate != 0 {
			b = append(b, '_')
		}

		b = append(b,
			byte(h/10)+'0',
			byte(h%10)+'0',
			':',
			byte(m/10)+'0',
			byte(m%10)+'0',
			':',
			byte(s/10)+'0',
			byte(s%10)+'0',
		)
	}
	if w.Flags&(Lmilliseconds|Lmicroseconds) != 0 {
		if w.Flags&(Ldate|Ltime) != 0 {
			b = append(b, '.')
		}

		ns := t.Nanosecond() / 1e3
		if w.Flags&Lmilliseconds != 0 {
			ns /= 1000

			b = append(b,
				byte(ns/100%10)+'0',
				byte(ns/10%10)+'0',
				byte(ns/1%10)+'0',
			)
		} else {
			b = append(b,
				byte(ns/100000%10)+'0',
				byte(ns/10000%10)+'0',
				byte(ns/1000%10)+'0',
				byte(ns/100%10)+'0',
				byte(ns/10%10)+'0',
				byte(ns/1%10)+'0',
			)
		}
	}

	if w.Colorize && len(w.TimeChangeColor) != 0 {
		c := common(b[ts:], w.lasttime)
		ts += c
		w.lasttime = append(w.lasttime[:c], b[ts:]...)

		if c != 0 && ts != len(b) {
			b = append(b, w.TimeChangeColor...)

			copy(b[ts+len(w.TimeChangeColor):], b[ts:])
			copy(b[ts:], w.TimeChangeColor)
		}
	}

	if w.Colorize && (len(w.TimeColor) != 0 || len(w.TimeChangeColor) != 0) {
		b = append(b, ResetColor...)
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

	b, i = w.ConvertValue(b, p, i, 0)

	vw := len(b) - vst

	// NOTE: Value width can be incorrect for non-ascii symbols.
	// We can calc it by iterating utf8.DecodeRune() but should we?

	if w.Colorize && len(w.ValColor) != 0 {
		b = append(b, ResetColor...)
	}

	nw := w.pad[tlow.UnsafeBytesToString(k)]

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

func (w *ConsoleWriter) ConvertValue(b, p []byte, st, ff int) (_ []byte, i int) {
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

		if tag == tlwire.Bytes && w.StringOnNewLineMinLen != 0 && len(s) >= w.StringOnNewLineMinLen {
			b = append(b, '\\', '\n')

			h := hex.Dumper(noescapeByteWriter(&b))

			_, _ = h.Write(s)
			_ = h.Close()

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

		if w.StringOnNewLineMinLen != 0 && len(s) >= w.StringOnNewLineMinLen {
			b = append(b, '\\', '\n')
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
			ss := tlow.UnsafeBytesToString(s)
			b = strconv.AppendQuote(b, ss)
		} else {
			b = append(b, s...)
		}

		if w.StringOnNewLineMinLen != 0 && len(s) >= w.StringOnNewLineMinLen && s[len(s)-1] != '\n' {
			b = append(b, '\n')
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

			b, i = w.ConvertValue(b, p, i, ff)
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

			b, i = w.ConvertValue(b, p, i, ff)

			b = append(b, ':')

			b, i = w.ConvertValue(b, p, i, ff)
		}

		b = append(b, '}')
	case tlwire.Special:
		switch sub {
		case tlwire.False:
			b = append(b, "false"...)
		case tlwire.True:
			b = append(b, "true"...)
		case tlwire.Nil:
			b = append(b, "<nil>"...)
		case tlwire.Undefined:
			b = append(b, "<undef>"...)
		case tlwire.None:
			// none
		case tlwire.Hidden:
			b = append(b, "<hidden>"...)
		case tlwire.SelfRef:
			b = append(b, "<self_ref>"...)
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
			var d time.Duration
			d, i = w.d.Duration(p, st)

			switch {
			case w.DurationFormat != "" && w.DurationDiv != 0:
				b = hfmt.Appendf(b, w.DurationFormat, float64(d/w.DurationDiv))
			case w.DurationFormat != "":
				b = hfmt.Appendf(b, w.DurationFormat, d)
			default:
				b = w.AppendDuration(b, d)
			}
		case WireID:
			var id ID
			i = id.TlogParse(p, st)

			st := len(b)
			b = append(b, "123456789_123456789_123456789_12"[:w.IDWidth]...)
			id.FormatTo(b[st:], 'v')
		case tlwire.Hex:
			b, i = w.ConvertValue(b, p, i, ff|cfHex)
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
			b, i = w.ConvertValue(b, p, i, ff)
		}
	default:
		panic(tag)
	}

	return b, i
}

func (w *ConsoleWriter) AppendDuration(b []byte, d time.Duration) []byte {
	if d == 0 {
		return append(b, ' ', ' ', '0', 's')
	}

	var buf [32]byte

	if d >= 99*time.Second {
		const MaxGroups = 2
		group := 0
		i := 0

		if d < 0 {
			d = -d
			b = append(b, '-')
		}

		add := func(d, unit time.Duration, suff byte) time.Duration {
			if group == 0 && d < unit && unit > time.Second || group >= MaxGroups {
				return d
			}

			x := int(d / unit)
			d = d % unit
			group++

			if group == MaxGroups && d >= unit/2 {
				x++
			}

			w := width(x)
			i += w
			for j := 1; j <= w; j++ {
				buf[i-j] = byte(x%10) + '0'
				x /= 10
			}

			buf[i] = suff
			i++

			return d
		}

		d = add(d, 24*time.Hour, 'd')
		d = add(d, time.Hour, 'h')
		d = add(d, time.Minute, 'm')
		d = add(d, time.Second, 's')

		return append(b, buf[:i]...)
	}

	neg := d < 0
	if neg {
		d = -d
	}

	end := len(buf) - 4
	i := end

	for d != 0 {
		i--
		buf[i] = byte(d%10) + '0'
		d /= 10
	}

	buf[i-1] = '0' // leading zero for possible carry

	j := i + 3
	buf[j] += 5 // round

	for j >= i-1 && buf[j] > '9' { // move carry
		buf[j] = '0'
		j--
		buf[j]++
	}

	j = end
	u := -1

	for buf[j-2] != 0 || buf[j-1] == '1' { // find suitable units
		j -= 3
		u++
	}

	i = j // beginning

	for buf[j] == '0' || buf[j] == 0 { // leading spaces
		buf[j] = ' '
		j++
	}

	digit := j

	j += 3

	// insert point
	if j-1 > i+3 {
		buf[j-1] = buf[j-2]
	}
	if j-2 > i+3 {
		buf[j-2] = buf[j-3]
	}

	buf[i+3] = '.'

	if j > end {
		j = end
	}

	for j > i+3 && (buf[j-1] == '0' || buf[j-1] == '.') { // trailing zeros
		j--
	}

	suff := []string{"ns", "µs", "ms", "s", "m"}
	j += copy(buf[j:], suff[u])

	if neg {
		buf[digit-1] = '-'

		if digit == i {
			i--
		}
	}

	return append(b, buf[i:j]...)
}

func width(n int) (w int) {
	q := 10
	w = 1

	for q <= n {
		w++
		q *= 10
	}

	return w
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

func common(x, y []byte) (n int) {
	for n < len(y) && x[n] == y[n] {
		n++
	}

	return
}

func noescapeByteWriter(b *[]byte) *low.Buf {
	//	return (*low.Buf)(b)
	return (*low.Buf)(noescape(unsafe.Pointer(b)))
}
