package tlog

import "github.com/nikandfor/tlog/wire"

const (
	WireLabels = wire.SemanticExtBase + iota
	WireID
	WireMessage
	WireEventType

	WireLogLevel
)

var KeyAuto = ""

var (
	_l  LogLevel
	_e  EventType
	_h  Hex
	_m  Message
	_ts Timestamp

	_, _, _, _, _, _, _, _, _ wire.TlogAppender = ID{}, Info, EventValue, Labels{}, Hex(0), Message(""), Timestamp(0), RawMessage{}, Format{}
	_, _, _, _, _, _, _, _    wire.TlogParser   = &ID{}, &_l, &_e, &Labels{}, &_h, &_m, &_ts, &RawMessage{}
)

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
				k = autoKey(kvs[i:])
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
		case FormatNext:
			i++
			if i == len(kvs) {
				b = append(b, wire.Special|wire.Undefined)
				break
			}

			b = append(b, wire.Semantic|WireMessage)
			b = e.AppendFormat(b, string(v), kvs[i])
		default:
			b = e.AppendValue(b, v)
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
	case Message:
		k = KeyMessage
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
	return e.AppendFormat(b, f.Fmt, f.Args...)
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
	b = append(b, wire.Semantic|wire.Hex)
	return e.AppendInt(b, wire.Int, uint64(x))
}

func (x *Hex) TlogParse(d *wire.Decoder, p []byte, i int) int {
	if p[i] != wire.Semantic|wire.Hex {
		panic("not a hex type")
	}

	i++

	v, i := d.Signed(p, i)

	*x = Hex(v)

	return i
}

func (m Message) TlogAppend(e *wire.Encoder, b []byte) []byte {
	b = append(b, wire.Semantic|WireMessage)
	return e.AppendString(b, wire.String, string(m))
}

func (m *Message) TlogParse(d *wire.Decoder, p []byte, i int) int {
	if p[i] != wire.Semantic|WireMessage {
		panic("not a message")
	}

	i++

	v, i := d.String(p, i)

	*m = Message(v)

	return i
}

func (ts Timestamp) TlogAppend(e *wire.Encoder, b []byte) []byte {
	b = append(b, wire.Semantic|wire.Time)
	return e.AppendInt(b, wire.Int, uint64(ts))
}

func (ts *Timestamp) TlogParse(d *wire.Decoder, p []byte, i int) int {
	if p[i] != wire.Semantic|wire.Time {
		panic("not a time")
	}

	i++

	v, i := d.Int(p, i)

	*ts = Timestamp(v)

	return i
}

func (r RawMessage) TlogAppend(e *wire.Encoder, b []byte) []byte {
	return append(b, r...)
}

func (r *RawMessage) TlogParse(d *wire.Decoder, p []byte, i int) int {
	end := d.Skip(p, i)
	*r = append((*r)[:0], p[i:end]...)
	return end
}
