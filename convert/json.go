package convert

import (
	"encoding/base64"
	"io"
	"path/filepath"
	"strconv"
	"time"

	"github.com/nikandfor/loc"
	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/low"
	"github.com/nikandfor/tlog/wire"
)

type (
	JSON struct {
		io.Writer

		AttachLabels bool
		TimeFormat   string
		TimeZone     *time.Location

		d wire.Decoder

		ls []byte

		b low.Buf
	}
)

func NewJSONWriter(w io.Writer) *JSON {
	return &JSON{
		Writer:     w,
		TimeFormat: time.RFC3339Nano,
		TimeZone:   time.Local,
	}
}

func (w *JSON) Write0(p []byte) (n int, err error) { return 0, nil }

func (w *JSON) Write(p []byte) (i int, err error) {
	b := w.b[:0]

	var e tlog.EventType
	var ls []byte

	tag, els, i := w.d.Tag(p, i)
	if tag != wire.Map {
		panic("not a map")
	}

	b = append(b, '{')

	var k []byte
	var j int
	for el := 0; els == -1 || el < int(els); el++ {
		if els == -1 && w.d.Break(p, &i) {
			break
		}

		if el != 0 {
			b = append(b, ',')
		}

		st := i
		bst := len(b)

		b, i = w.appendValue(b, p, i, 0)

		b = append(b, ':')

		b, i = w.appendValue(b, p, i, 0)

		// semantic

		k, j = w.d.String(p, st)

		tag, sub, _ := w.d.Tag(p, j)
		if tag != wire.Semantic {
			continue
		}

		switch {
		case sub == tlog.WireEventType && string(k) == tlog.KeyEventType:
			e.TlogParse(&w.d, p, j)
		case sub == tlog.WireLabels && string(k) == tlog.KeyLabels:
			ls = b[bst:]
		}
	}

	if e == tlog.EventLabels {
		w.ls = append(w.ls[:0], ls...)
	} else if w.AttachLabels && len(w.ls) != 0 {
		if len(b) > 1 {
			b = append(b, ',')
		}

		b = append(b, w.ls...)
	}

	b = append(b, '}', '\n')

	w.b = b[:0]

	_, err = w.Writer.Write(b)
	if err != nil {
		return 0, err
	}

	return len(p), nil
}

func (w *JSON) appendValue(b, p []byte, st int, d int) (_ []byte, i int) {
	tag, sub, i := w.d.Tag(p, st)

	var s []byte
	var f float64

	switch tag {
	case wire.Int:
		var v uint64
		v, i = w.d.Int(p, st)

		b = strconv.AppendUint(b, v, 10)
	case wire.Neg:
		var v uint64
		v, i = w.d.Int(p, st)

		b = strconv.AppendInt(b, -int64(v), 10)
	case wire.Bytes:
		s, i = w.d.String(p, st)

		b = append(b, '"')

		m := base64.StdEncoding.EncodedLen(len(s))
		d := len(b)

		for cap(b)-d < m {
			b = append(b[:cap(b)], 0, 0, 0, 0)
		}

		b = b[:d+m]

		base64.StdEncoding.Encode(b[d:], s)

		b = append(b, '"')
	case wire.String:
		s, i = w.d.String(p, st)

		b = low.AppendQuote(b, low.UnsafeBytesToString(s))

	case wire.Array:
		b = append(b, '[')

		for el := 0; sub == -1 || el < int(sub); el++ {
			if sub == -1 && w.d.Break(p, &i) {
				break
			}

			if el != 0 {
				b = append(b, ',')
			}

			b, i = w.appendValue(b, p, i, d+1)
		}

		b = append(b, ']')
	case wire.Map:
		b = append(b, '{')

		for el := 0; sub == -1 || el < int(sub); el++ {
			if sub == -1 && w.d.Break(p, &i) {
				break
			}

			if el != 0 {
				b = append(b, ',')
			}

			b, i = w.appendValue(b, p, i, d+1)

			b = append(b, ':')

			b, i = w.appendValue(b, p, i, d+1)
		}

		b = append(b, '}')
	case wire.Semantic:
		switch {
		case sub == wire.Time:
			var t time.Time
			t, i = w.d.Time(p, st)

			if w.TimeZone != nil {
				t = t.In(w.TimeZone)
			}

			b = append(b, '"')
			b = t.AppendFormat(b, w.TimeFormat)
			b = append(b, '"')
		case sub == tlog.WireID:
			var id tlog.ID
			i = id.TlogParse(&w.d, p, st)

			bst := len(b) + 1
			b = append(b, `"123456789_123456789_123456789_12"`...)

			id.FormatTo(b[bst:], 'x')
		case sub == wire.Caller:
			var pc loc.PC
			pc, i = w.d.Caller(p, st)

			_, file, line := pc.NameFileLine()

			b = low.AppendPrintf(b, `"%v:%d"`, filepath.Base(file), line)
		default:
			b, i = w.appendValue(b, p, i, d+1)
		}
	case wire.Special:
		switch sub {
		case wire.False:
			b = append(b, "false"...)
		case wire.True:
			b = append(b, "true"...)
		case wire.Null, wire.Undefined:
			b = append(b, "null"...)
		case wire.Float64, wire.Float32, wire.Float16, wire.Float8:
			f, i = w.d.Float(p, st)

			b = strconv.AppendFloat(b, f, 'f', -1, 64)
		default:
			panic(sub)
		}
	}

	return b, i
}
