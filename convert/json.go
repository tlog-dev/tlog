package convert

import (
	"encoding/base64"
	"io"
	"strconv"
	"time"

	"github.com/nikandfor/loc"
	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/low"
)

type (
	JSON struct {
		io.Writer

		TimeFormat   string
		TimeInUTC    bool
		AttachLabels bool

		d tlog.Decoder

		ls, tmpls tlog.Labels

		b low.Buf
	}
)

func NewJSONWriter(w io.Writer) *JSON {
	return &JSON{
		Writer: w,
		//	TimeFormat: "2006-01-02T15:04:05.999999Z07:00",
	}
}

func (w *JSON) Write(p []byte) (n int, err error) {
	w.d.ResetBytes(p)

	b := w.b[:0]
	i := 0

	for i < len(p) {
		b, i = w.appendValue(b, i, 0, nil)

		b = append(b, '\n')
	}

	w.b = b[:0]

	if err = w.d.Err(); err != nil {
		return 0, err
	}

	_, err = w.Writer.Write(b)
	if err != nil {
		return 0, err
	}

	if w.tmpls != nil {
		w.ls = w.tmpls
		w.tmpls = nil
	}

	return len(p), nil
}

func (w *JSON) appendValue(b []byte, st, d int, key []byte) (_ []byte, i int) {
	tag, sub, i := w.d.Tag(st)
	if w.d.Err() != nil {
		return
	}

	var v int64
	var s []byte
	var f float64

	switch tag {
	case tlog.Int:
		v, i = w.d.Int(st)

		b = strconv.AppendUint(b, uint64(v), 10)
	case tlog.Neg:
		v, i = w.d.Int(st)

		b = strconv.AppendInt(b, v, 10)
	case tlog.Bytes:
		s, i = w.d.String(st)

		b = append(b, '"')

		m := base64.StdEncoding.EncodedLen(len(s))
		d := len(b)

		for cap(b)-d < m {
			b = append(b[:cap(b)], 0, 0, 0, 0)
		}

		b = b[:d+m]

		base64.StdEncoding.Encode(b[d:], s)

		b = append(b, '"')
	case tlog.String:
		s, i = w.d.String(st)

		b = low.AppendQuote(b, low.UnsafeBytesToString(s))

	case tlog.Array:
		b = append(b, '[')

		for el := 0; sub == -1 || el < sub; el++ {
			if sub == -1 && w.d.Break(&i) {
				break
			}

			if el != 0 {
				b = append(b, ',')
			}

			b, i = w.appendValue(b, i, d+1, nil)
		}

		b = append(b, ']')
	case tlog.Map:
		b = append(b, '{')

		for el := 0; sub == -1 || el < sub; el++ {
			if sub == -1 && w.d.Break(&i) {
				break
			}

			if el != 0 {
				b = append(b, ',')
			}

			kst := len(b)
			b, i = w.appendValue(b, i, d+1, nil)

			var key []byte
			if l := len(b) - 1; b[kst] == '"' && b[l] == '"' && kst+1 < l {
				key = b[kst+1 : l]
			}

			b = append(b, ':')

			b, i = w.appendValue(b, i, d+1, key)
		}

		if d == 0 && len(w.ls) != 0 && w.AttachLabels {
			if i != st {
				b = append(b, ',')
			}
			b = append(b, `"L":[`...)
			for el, l := range w.ls {
				if el != 0 {
					b = append(b, ',')
				}

				b = low.AppendQuote(b, l)
			}
			b = append(b, ']')
		}

		b = append(b, '}')
	case tlog.Semantic:
		if key == nil {
			b, i = w.appendValue(b, i, d+1, key)
			break
		}

		ks := low.UnsafeBytesToString(key)

		switch {
		case sub == tlog.WireTime && w.TimeFormat != "" && ks == tlog.KeyTime:
			var ts tlog.Timestamp
			ts, i = w.d.Time(st)

			t := time.Unix(0, int64(ts))
			if w.TimeInUTC {
				t = t.UTC()
			}

			b = append(b, '"')
			b = t.AppendFormat(b, w.TimeFormat)
			b = append(b, '"')
		case sub == tlog.WireID:
			var id tlog.ID
			id, i = w.d.ID(st)

			bst := len(b) + 1
			b = append(b, `"123456789_123456789_123456789_12"`...)

			id.FormatTo(b[bst:], 'x')
		case sub == tlog.WireLocation:
			var pc loc.PC
			var pcs loc.PCs

			pc, pcs, i = w.d.Location(st)

			if pcs == nil {
				b = low.AppendQuote(b, pc.String())
			} else {
				b = append(b, '[')
				for i, pc := range pcs {
					if i != 0 {
						b = append(b, ',')
					}

					b = low.AppendQuote(b, pc.String())
				}
				b = append(b, ']')
			}
		case sub == tlog.WireLabels && ks == tlog.KeyLabels:
			vst := i

			var ls tlog.Labels
			ls, i = w.d.Labels(st)

			w.tmpls = ls

			b, i = w.appendValue(b, vst, d+1, key)
		default:
			b, i = w.appendValue(b, i, d+1, key)
		}
	case tlog.Special:
		switch sub {
		case tlog.False:
			b = append(b, "false"...)
		case tlog.True:
			b = append(b, "true"...)
		case tlog.Null, tlog.Undefined:
			b = append(b, "null"...)
		case tlog.Float64, tlog.Float32, tlog.Float16, tlog.FloatInt8:
			f, i = w.d.Float(st)

			b = strconv.AppendFloat(b, f, 'f', -1, 64)
		default:
			panic(sub)
		}
	}

	return b, i
}
