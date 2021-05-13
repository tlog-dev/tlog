package tlog

import "github.com/nikandfor/tlog/wire"

const (
	WireLabels = wire.SemanticExtBase + iota
	WireID
	WireMessage
	WireEventType
	WireLogLevel

	WireHex
)

var KeyAuto = ""

func AppendKVs(e *wire.Encoder, b []byte, kvs []interface{}) []byte {
	return appendKVs0(e, b, kvs)
}

func appendKVs(e *wire.Encoder, b []byte, kvs []interface{}) []byte {
	for i := 0; i < len(kvs); {
		k, ok := kvs[i].(string)
		if !ok {
			k = "MISSING_KEY"
		} else {
			if k == KeyAuto {
				k = autoKey(kvs)
			}

			i++
		}

		b = e.AppendString(b, wire.String, k)

		if i == len(kvs) {
			b = append(b, wire.Special|wire.Undefined)
			break
		}

		switch v := kvs[i].(type) {
		case string:
			b = e.AppendString(b, wire.String, v)
		case int:
			b = e.AppendSigned(b, int64(v))
		default:
			b = e.AppendValue(b, v)
		case FormatNext:
			i++
			if i == len(kvs) {
				b = append(b, wire.Special|wire.Undefined)
				break
			}

			b = append(b, wire.Semantic|WireMessage)
			b = e.AppendFormat(b, string(v), kvs[i])
		}

		i++
	}

	return b
}

func autoKey(kvs []interface{}) (k string) {
	if len(kvs) == 1 {
		return "MISSING_VALUE"
	}

	switch kvs[1].(type) {
	case ID:
		k = KeySpan
	case LogLevel:
		k = KeyLogLevel
	case EventType:
		k = KeyEventType
	case Labels:
		k = KeyLabels
	default:
		k = "UNSUPPORTED_AUTO_KEY"
	}

	return
}

func (id ID) TlogAppend(e *wire.Encoder, b []byte) []byte {
	b = append(b, wire.Semantic|WireID)
	return e.AppendStringBytes(b, wire.Bytes, id[:])
}

func (id *ID) TlogParse(d *wire.Decoder, p []byte, i int) int {
	if p[i] != wire.Semantic|WireID {
		panic("not an id")
	}

	i++

	if p[i] != wire.Bytes|16 {
		panic("not an id")
	}

	i++

	i += copy((*id)[:], p[i:])

	return i
}

func (l LogLevel) TlogAppend(e *wire.Encoder, b []byte) []byte {
	b = append(b, wire.Semantic|WireLogLevel)
	return e.AppendSigned(b, int64(l))
}

func (l *LogLevel) TlogParse(d *wire.Decoder, p []byte, i int) int {
	if p[i] != wire.Semantic|WireLogLevel {
		panic("not a log level")
	}

	i++

	v, i := d.Signed(p, i)

	*l = LogLevel(v)

	return i
}

func (et EventType) TlogAppend(e *wire.Encoder, b []byte) []byte {
	b = append(b, wire.Semantic|WireEventType)
	return e.AppendString(b, wire.String, string(et))
}

func (e *EventType) TlogParse(d *wire.Decoder, p []byte, i int) int {
	if p[i] != wire.Semantic|WireEventType {
		panic("not an event type")
	}

	i++

	v, i := d.String(p, i)

	*e = EventType(v[0])

	return i
}

func (f Format) TlogAppend(e *wire.Encoder, b []byte) []byte {
	return e.AppendFormat(b, f.Fmt, f.Args)
}

func (ls Labels) TlogAppend(e *wire.Encoder, b []byte) []byte {
	b = append(b, wire.Semantic|WireLabels)
	b = e.AppendArray(b, len(ls))

	for _, l := range ls {
		b = e.AppendString(b, wire.String, l)
	}

	return b
}

func (ls *Labels) TlogParse(d *wire.Decoder, p []byte, i int) int {
	if p[i] != wire.Semantic|WireLabels {
		panic("not labels")
	}

	i++

	_, els, i := d.Tag(p, i)

	*ls = (*ls)[:0]

	var v []byte
	for el := 0; el < int(els); el++ {
		v, i = d.String(p, i)

		*ls = append(*ls, string(v))
	}

	return i
}

func (x Hex) TlogAppend(e *wire.Encoder, b []byte) []byte {
	b = append(b, wire.Semantic|WireHex)
	return e.AppendSigned(b, int64(x))
}

func (x *Hex) TlogParse(d *wire.Decoder, p []byte, i int) int {
	if p[i] != wire.Semantic|WireHex {
		panic("not a hex type")
	}

	i++

	v, i := d.Signed(p, i)

	*x = Hex(v)

	return i
}
