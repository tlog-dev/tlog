package convert

import (
	"encoding/base64"
	"errors"
	"io"
	"math"
	"path/filepath"
	"strconv"
	"time"

	"nikand.dev/go/hacked/hfmt"
	"nikand.dev/go/hacked/low"
	"tlog.app/go/loc"

	"tlog.app/go/tlog"
	tlow "tlog.app/go/tlog/low"
	"tlog.app/go/tlog/tlwire"
)

type (
	JSON struct {
		io.Writer

		AppendNewLine bool
		AppendKeySafe bool
		FloatInfNaN   bool
		TimeFormat    string
		TimeZone      *time.Location

		Rename RenameFunc

		d tlwire.Decoder

		b low.Buf
	}

	RenameFunc func(b, p, k []byte, st int) ([]byte, bool)

	TagSub struct {
		Tag tlwire.Tag
		Sub int64
	}

	SimpleRenameRule struct {
		Tags []TagSub
		Key  string
	}

	SimpleRenamer struct {
		tlwire.Decoder

		Rules map[string]SimpleRenameRule

		Fallback RenameFunc
	}
)

func NewJSON(w io.Writer) *JSON {
	return &JSON{
		Writer:        w,
		AppendNewLine: true,
		AppendKeySafe: true,
		FloatInfNaN:   false,
		TimeFormat:    time.RFC3339Nano,
		TimeZone:      time.Local,
	}
}

func (w *JSON) Write(p []byte) (i int, err error) {
	b := w.b[:0]

more:
	tag, els, i := w.d.Tag(p, i)
	if tag != tlwire.Map {
		return i, errors.New("map expected")
	}

	b = append(b, '{')

	var k []byte
	for el := 0; els == -1 || el < int(els); el++ {
		if els == -1 && w.d.Break(p, &i) {
			break
		}

		if el != 0 {
			b = append(b, ',')
		}

		b = append(b, '"')

		k, i = w.d.Bytes(p, i)

		var renamed bool

		if w.Rename != nil {
			b, renamed = w.Rename(b, p, k, i)
		}

		if !renamed {
			if w.AppendKeySafe {
				b = tlow.AppendSafe(b, k)
			} else {
				b = append(b, k...)
			}
		}

		b = append(b, '"', ':')

		b, i = w.ConvertValue(b, p, i)
	}

	b = append(b, '}')
	if w.AppendNewLine {
		b = append(b, '\n')
	}

	if i < len(p) {
		goto more
	}

	w.b = b[:0]

	_, err = w.Writer.Write(b)
	if err != nil {
		return 0, err
	}

	return len(p), nil
}

func (w *JSON) ConvertKey(b, p []byte, st int) (_ []byte, i int) {
	tag := w.d.TagOnly(p, st)

	b = append(b, '"')

	switch tag {
	case tlwire.Int, tlwire.Neg,
		tlwire.Special:
		b, i = w.ConvertValue(b, p, st)
	case tlwire.Bytes, tlwire.String:
		var k []byte
		k, i = w.d.Bytes(p, st)

		if w.AppendKeySafe {
			b = tlow.AppendSafe(b, k)
		} else {
			b = append(b, k...)
		}
	default:
		b = hfmt.Appendf(b, `UNSUPPORTED_KEY_TYPE_%x`, tag)
		i = w.d.Skip(p, st)
	}

	b = append(b, '"')

	return b, i
}

func (w *JSON) ConvertValue(b, p []byte, st int) (_ []byte, i int) {
	tag, sub, i := w.d.Tag(p, st)

	switch tag {
	case tlwire.Int:
		b = strconv.AppendUint(b, uint64(sub), 10)
	case tlwire.Neg:
		b = strconv.AppendInt(b, -sub-1, 10)
	case tlwire.Bytes:
		b = append(b, '"')

		m := base64.StdEncoding.EncodedLen(int(sub))
		bst := len(b)

		for cap(b) < bst+m {
			b = append(b[:cap(b)], 0, 0, 0, 0)
		}

		b = b[:bst+m]

		base64.StdEncoding.Encode(b[bst:], p[i:i+int(sub)])

		b = append(b, '"')

		i += int(sub)
	case tlwire.String:
		b = append(b, '"')

		b = tlow.AppendSafe(b, p[i:i+int(sub)])

		b = append(b, '"')

		i += int(sub)
	case tlwire.Array:
		b = append(b, '[')

		for el := 0; sub == -1 || el < int(sub); el++ {
			if sub == -1 && w.d.Break(p, &i) {
				break
			}

			if el != 0 {
				b = append(b, ',')
			}

			b, i = w.ConvertValue(b, p, i)
		}

		b = append(b, ']')
	case tlwire.Map:
		b = append(b, '{')

		for el := 0; sub == -1 || el < int(sub); el++ {
			if sub == -1 && w.d.Break(p, &i) {
				break
			}

			if el != 0 {
				b = append(b, ',')
			}

			b, i = w.ConvertKey(b, p, i)

			b = append(b, ':')

			b, i = w.ConvertValue(b, p, i)
		}

		b = append(b, '}')
	case tlwire.Semantic:
		switch sub {
		case tlwire.Time:
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
			i = id.TlogParse(p, st)

			bst := len(b) + 1
			b = append(b, `"12345678-9_12-3456-789_-123456789_12"`...)

			id.FormatTo(b, bst, 'u')
		case tlwire.Caller:
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
					b = hfmt.Appendf(b, `"%v:%d"`, filepath.Base(file), line)
				}
				b = append(b, ']')
			} else {
				_, file, line := pc.NameFileLine()

				b = hfmt.Appendf(b, `"%v:%d"`, filepath.Base(file), line)
			}
		default:
			b, i = w.ConvertValue(b, p, i)
		}
	case tlwire.Special:
		switch sub {
		case tlwire.False:
			b = append(b, "false"...)
		case tlwire.True:
			b = append(b, "true"...)
		case tlwire.Nil, tlwire.Undefined, tlwire.None, tlwire.Hidden, tlwire.SelfRef:
			b = append(b, "null"...)
		case tlwire.Float64, tlwire.Float32, tlwire.Float16, tlwire.Float8:
			var f float64
			f, i = w.d.Float(p, st)

			switch {
			case !w.FloatInfNaN && math.IsNaN(f):
				b = append(b, `"NaN"`...)
			case !w.FloatInfNaN && math.IsInf(f, 1):
				b = append(b, `"+Inf"`...)
			case !w.FloatInfNaN && math.IsInf(f, -1):
				b = append(b, `"-Inf"`...)
			default:
				b = strconv.AppendFloat(b, f, 'f', -1, 64)
			}

		default:
			panic(sub)
		}
	}

	return b, i
}

func (r SimpleRenamer) Rename(b, p, k []byte, i int) ([]byte, bool) {
	rule, ok := r.Rules[string(k)]
	if !ok {
		return r.fallback(b, p, k, i)
	}

	for _, ts := range rule.Tags {
		tag, sub, j := r.Tag(p, i)

		if tag != tlwire.Semantic && tag != tlwire.Special {
			sub = 0
		}

		if ts != (TagSub{tag, sub}) {
			return r.fallback(b, p, k, i)
		}

		i = j
	}

	return append(b, rule.Key...), true
}

func (r SimpleRenamer) fallback(b, p, k []byte, i int) ([]byte, bool) {
	if r.Fallback == nil {
		return b, false
	}

	return r.Fallback(b, p, k, i)
}
