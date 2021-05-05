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

		AttachLabels bool
		TimeFormat   string
		TimeZone     *time.Location

		d tlog.Decoder

		ls []byte

		b low.Buf
	}
)

func NewJSONWriter(w io.Writer) *JSON {
	return &JSON{
		Writer: w,
		//	TimeFormat: "2006-01-02T15:04:05.999999Z07:00",
		TimeZone: time.Local,
	}
}

func (w *JSON) Write(p []byte) (n int, err error) {
	defer w.d.ResetBytes(nil)
	w.d.ResetBytes(p)

	var i, j int64
	b := w.b[:0]

	var e tlog.EventType
	var ls []byte

	tag, els, i := w.d.Tag(i)
	if w.d.Err() != nil {
		return
	}

	if tag != tlog.Map {
		panic("not a map")
	}

	b = append(b, '{')

	var k []byte
	for el := 0; els == -1 || el < els; el++ {
		if els == -1 && w.d.Break(&i) {
			break
		}

		if el != 0 {
			b = append(b, ',')
		}

		st := i
		bst := len(b)

		b, i = w.appendValue(b, i, 0)

		b = append(b, ':')

		b, i = w.appendValue(b, i, 0)

		k, j = w.d.String(st)

		tag, sub, _ := w.d.Tag(j)
		if tag != tlog.Semantic {
			continue
		}

		switch {
		case sub == tlog.WireEventType && string(k) == tlog.KeyEventType:
			e, _ = w.d.EventType(j)
		case sub == tlog.WireLabels && string(k) == tlog.KeyLabels:
			ls = b[bst:]
		}
	}

	if e == tlog.EventLabels {
		w.ls = append(w.ls[:0], ls...)
	} else if w.AttachLabels {
		if len(b) > 1 {
			b = append(b, ',')
		}

		b = append(b, w.ls...)
	}

	b = append(b, '}', '\n')

	w.b = b[:0]

	if err = w.d.Err(); err != nil {
		return 0, err
	}

	_, err = w.Writer.Write(b)
	if err != nil {
		return 0, err
	}

	return len(p), nil
}

func (w *JSON) appendValue(b []byte, st int64, d int) (_ []byte, i int64) {
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

			b, i = w.appendValue(b, i, d+1)
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

			b, i = w.appendValue(b, i, d+1)

			b = append(b, ':')

			b, i = w.appendValue(b, i, d+1)
		}

		b = append(b, '}')
	case tlog.Semantic:
		switch {
		case sub == tlog.WireTime && w.TimeFormat != "":
			var ts tlog.Timestamp
			ts, i = w.d.Time(st)

			t := time.Unix(0, int64(ts))
			if w.TimeZone != nil {
				t = t.In(w.TimeZone)
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
		case sub == tlog.WireCaller:
			var pc loc.PC
			var pcs loc.PCs

			pc, pcs, i = w.d.Caller(st)

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
		default:
			b, i = w.appendValue(b, i, d+1)
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
