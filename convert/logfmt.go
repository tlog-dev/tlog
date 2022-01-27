package convert

import (
	"errors"
	"io"
	"path/filepath"
	"strconv"
	"time"

	"github.com/nikandfor/loc"
	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/low"
	"github.com/nikandfor/tlog/tlio"
	"github.com/nikandfor/tlog/wire"
	"golang.org/x/term"
)

type (
	Logfmt struct {
		io.Writer

		TimeFormat string
		TimeZone   *time.Location

		FloatFormat    string
		FloatChar      byte
		FloatPrecision int

		QuoteChars string

		PairSeparator  string
		KVSeparator    string
		ArrSeparator   string
		MapSeparator   string
		MapKVSeparator string

		MaxValPad int

		AppendKeySafe bool

		Colorize bool
		KeyColor []byte
		ValColor []byte

		d wire.Decoder

		b low.Buf

		addpad int
		pad    map[string]int
	}
)

func NewLogfmt(w io.Writer) *Logfmt {
	fd := tlio.Fd(w)
	colorize := term.IsTerminal(int(fd))

	return &Logfmt{
		Writer: w,

		TimeFormat:     "2006-01-02T15:04:05.000000000Z07:00",
		TimeZone:       time.Local,
		FloatChar:      'f',
		FloatPrecision: 5,
		QuoteChars:     "`\"' ()[]{}*",

		PairSeparator:  "  ",
		KVSeparator:    "=",
		ArrSeparator:   " ",
		MapSeparator:   " ",
		MapKVSeparator: "=",

		MaxValPad: 24,

		Colorize: colorize,
		KeyColor: tlog.Color(36),

		AppendKeySafe: true,

		pad: make(map[string]int),
	}
}

func (w *Logfmt) Write(p []byte) (i int, err error) {
	tag, els, i := w.d.Tag(p, i)
	if tag != wire.Map {
		return i, errors.New("map expected")
	}

	w.addpad = 0

	b := w.b[:0]

	var k []byte
	for el := 0; els == -1 || el < int(els); el++ {
		if els == -1 && w.d.Break(p, &i) {
			break
		}

		k, i = w.d.String(p, i)

		b, i = w.appendPair(b, p, k, i)
	}

	b = append(b, '\n')

	w.b = b[:0]

	_, err = w.Writer.Write(b)
	if err != nil {
		return 0, err
	}

	return len(p), nil
}

func (w *Logfmt) appendPair(b, p, k []byte, st int) (_ []byte, i int) {
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

	if w.AppendKeySafe {
		b = low.AppendSafe(b, k)
	} else {
		b = append(b, k...)
	}

	b = append(b, w.KVSeparator...)

	if w.Colorize && len(w.ValColor) != 0 {
		b = append(b, w.ValColor...)
	} else if w.Colorize && len(w.KeyColor) != 0 {
		b = append(b, tlog.ResetColor...)
	}

	vst := len(b)

	b, i = w.convertValue(b, p, st, 0)

	vw := len(b) - vst

	// NOTE: Value width can be incorrect for non-ascii symbols.
	// We can calc it by iterating utf8.DecodeRune() but should we?

	if w.Colorize && len(w.ValColor) != 0 {
		b = append(b, tlog.ResetColor...)
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

func (w *Logfmt) convertValue(b, p []byte, st int, ff int) (_ []byte, i int) {
	tag, sub, i := w.d.Tag(p, st)

	switch tag {
	case wire.Int:
		b = strconv.AppendUint(b, uint64(sub), 10)
	case wire.Neg:
		b = strconv.AppendInt(b, sub, 10)
	case wire.Bytes, wire.String:
		var s []byte
		s, i = w.d.String(p, st)

		quote := tag == wire.Bytes || len(s) == 0
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
	case wire.Array:
		b = append(b, '[')

		for el := 0; sub == -1 || el < int(sub); el++ {
			if sub == -1 && w.d.Break(p, &i) {
				break
			}

			if el != 0 {
				b = append(b, w.ArrSeparator...)
			}

			b, i = w.convertValue(b, p, i, ff)
		}

		b = append(b, ']')
	case wire.Map:
		var k []byte

		b = append(b, '{')

		for el := 0; sub == -1 || el < int(sub); el++ {
			if sub == -1 && w.d.Break(p, &i) {
				break
			}

			if el != 0 {
				b = append(b, w.MapSeparator...)
			}

			k, i = w.d.String(p, i)

			if w.AppendKeySafe {
				b = low.AppendSafe(b, k)
			} else {
				b = append(b, k...)
			}

			b = append(b, w.MapKVSeparator...)

			b, i = w.convertValue(b, p, i, ff)
		}

		b = append(b, '}')
	case wire.Semantic:
		switch sub {
		case wire.Time:
			var t time.Time
			t, i = w.d.Time(p, st)

			if w.TimeZone != nil {
				t = t.In(w.TimeZone)
			}

			if w.TimeFormat != "" {
				b = append(b, '"')
				b = t.AppendFormat(b, w.TimeFormat)
				b = append(b, '"')
			} else {
				b = strconv.AppendInt(b, t.UnixNano(), 10)
			}
		case tlog.WireID:
			var id tlog.ID
			i = id.TlogParse(&w.d, p, st)

			bst := len(b) + 1
			b = append(b, `"123456789_123456789_123456789_12"`...)

			id.FormatTo(b[bst:], 'x')
		case wire.Caller:
			var pc loc.PC
			var pcs loc.PCs
			pc, pcs, i = w.d.Callers(p, st)

			if pcs != nil {
				b = append(b, '[')
				for i, pc := range pcs {
					if i != 0 {
						b = append(b, ',')
					}

					_, file, line := pc.NameFileLine()
					b = low.AppendPrintf(b, `"%v:%d"`, filepath.Base(file), line)
				}
				b = append(b, ']')
			} else {
				_, file, line := pc.NameFileLine()

				b = low.AppendPrintf(b, `"%v:%d"`, filepath.Base(file), line)
			}
		default:
			b, i = w.convertValue(b, p, i, ff)
		}
	case wire.Special:
		switch sub {
		case wire.False:
			b = append(b, "false"...)
		case wire.True:
			b = append(b, "true"...)
		case wire.Nil:
			b = append(b, "<nil>"...)
		case wire.Undefined:
			b = append(b, "<undef>"...)
		case wire.Float64, wire.Float32, wire.Float8:
			var f float64
			f, i = w.d.Float(p, st)

			if w.FloatFormat != "" {
				b = low.AppendPrintf(b, w.FloatFormat, f)
			} else {
				b = strconv.AppendFloat(b, f, w.FloatChar, w.FloatPrecision, 64)
			}
		default:
			panic(sub)
		}
	default:
		panic(tag)
	}

	return b, i
}
