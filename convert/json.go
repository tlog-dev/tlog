package convert

import (
	"encoding/base64"
	"io"
	"strconv"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/low"
)

type (
	JSON struct {
		io.Writer

		d tlog.Decoder

		b low.Buf
	}
)

func NewJSONWriter(w io.Writer) *JSON {
	return &JSON{
		Writer: w,
	}
}

func (w *JSON) Write(p []byte) (n int, err error) {
	w.d.ResetBytes(p)

	b := w.b[:0]
	i := 0

	for i < len(p) {
		b, i = w.appendValue(b, i)

		b = append(b, '\n')
	}

	w.b = b[:0]

	_, err = w.Writer.Write(b)
	if err != nil {
		return 0, err
	}

	return len(p), nil
}

func (w *JSON) appendValue(b []byte, st int) (_ []byte, i int) {
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

		b = strconv.AppendQuote(b, low.UnsafeBytesToString(s))

	case tlog.Array:
		b = append(b, '[')

		for el := 0; sub == -1 || el < sub; el++ {
			if sub == -1 && w.d.Break(&i) {
				break
			}

			if el != 0 {
				b = append(b, ',')
			}

			b, i = w.appendValue(b, i)
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

			b, i = w.appendValue(b, i)

			b = append(b, ':')

			b, i = w.appendValue(b, i)
		}

		b = append(b, '}')
	case tlog.Semantic:
		b, i = w.appendValue(b, i)
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
