package convert

import (
	"encoding/base64"
	"errors"
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

		AppendNewLine bool
		AppendSafe    bool
		TimeFormat    string
		TimeZone      *time.Location

		Rename map[KeyTagSub]string

		d wire.Decoder

		b low.Buf
	}

	KeyTagSub struct {
		Key string
		Tag byte
		Sub int64
	}
)

func NewJSONWriter(w io.Writer) *JSON {
	return &JSON{
		Writer:        w,
		AppendNewLine: true,
		AppendSafe:    true,
		TimeFormat:    time.RFC3339Nano,
		TimeZone:      time.Local,
	}
}

func (w *JSON) Write(p []byte) (i int, err error) {
	tag, els, i := w.d.Tag(p, i)
	if tag != wire.Map {
		return i, errors.New("map expected")
	}

	b := w.b[:0]

	b = append(b, '{')

	var k []byte
	var sub int64
	for el := 0; els == -1 || el < int(els); el++ {
		if els == -1 && w.d.Break(p, &i) {
			break
		}

		if el != 0 {
			b = append(b, ',')
		}

		b = append(b, '"')

		k, i = w.d.String(p, i)

		tag, sub, _ = w.d.Tag(p, i)

		var renamed bool

		if w.Rename != nil {
			kts := KeyTagSub{
				Key: low.UnsafeBytesToString(k),
				Tag: tag,
			}

			if tag == wire.Semantic || tag == wire.Special {
				kts.Sub = sub
			}

			var key string
			key, renamed = w.Rename[kts]

			if renamed {
				if w.AppendSafe {
					b = low.AppendSafeString(b, key)
				} else {
					b = append(b, key...)
				}
			}
		}

		if !renamed {
			if w.AppendSafe {
				b = low.AppendSafe(b, k)
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

	w.b = b[:0]

	_, err = w.Writer.Write(b)
	if err != nil {
		return 0, err
	}

	return len(p), nil
}

func (w *JSON) ConvertValue(b, p []byte, st int) (_ []byte, i int) {
	tag, sub, i := w.d.Tag(p, st)

	switch tag {
	case wire.Int:
		b = strconv.AppendUint(b, uint64(sub), 10)
	case wire.Neg:
		b = strconv.AppendInt(b, sub, 10)
	case wire.Bytes:
		b = append(b, '"')

		m := base64.StdEncoding.EncodedLen(int(sub))
		st := len(b)

		for st+m < cap(b) {
			b = append(b[:cap(b)], 0, 0, 0, 0)
		}

		b = b[:st+m]

		base64.StdEncoding.Encode(b[st:], p[i:])

		b = append(b, '"')

		i += int(sub)
	case wire.String:
		b = append(b, '"')

		if w.AppendSafe {
			b = low.AppendSafe(b, p[i:i+int(sub)])
		} else {
			b = append(b, p[i:i+int(sub)]...)
		}

		b = append(b, '"')

		i += int(sub)
	case wire.Array:
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
	case wire.Map:
		var k []byte

		b = append(b, '{')

		for el := 0; sub == -1 || el < int(sub); el++ {
			if sub == -1 && w.d.Break(p, &i) {
				break
			}

			if el != 0 {
				b = append(b, ',')
			}

			k, i = w.d.String(p, i)

			b = append(b, '"')

			if w.AppendSafe {
				b = low.AppendSafe(b, k)
			} else {
				b = append(b, k...)
			}

			b = append(b, '"', ':')

			b, i = w.ConvertValue(b, p, i)
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
			b, i = w.ConvertValue(b, p, i)
		}
	case wire.Special:
		switch sub {
		case wire.False:
			b = append(b, "false"...)
		case wire.True:
			b = append(b, "true"...)
		case wire.Nil, wire.Undefined:
			b = append(b, "null"...)
		case wire.Float64, wire.Float32, wire.Float16, wire.Float8:
			var f float64
			f, i = w.d.Float(p, st)

			b = strconv.AppendFloat(b, f, 'f', -1, 64)
		default:
			panic(sub)
		}
	}

	return b, i
}
