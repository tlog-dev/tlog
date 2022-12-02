package tlog

import (
	"unicode/utf8"
	_ "unsafe"

	"github.com/nikandfor/loc"
	"github.com/nikandfor/tlog/tlwire"
)

type (
	RawMessage []byte

	Modify []byte

	Timestamp int64

	FormatNext string

	format struct {
		Fmt  string
		Args []interface{}
	}
)

const KeyAuto = ""

var (
	None          = RawMessage{tlwire.Special | tlwire.None}
	NextIsHex     = Modify{tlwire.Semantic | tlwire.Hex}
	NextIsMessage = Modify{tlwire.Semantic | WireMessage}

//	None          = RawMessage(tlwire.LowEncoder{}.AppendSpecial(nil, tlwire.None))
//	HexNext       = SemanticNext(tlwire.Hex)
//	MessageNext   = SemanticNext(WireMessage)
//	NextIsHex     = Modify(tlwire.LowEncoder{}.AppendSemantic(nil, tlwire.Hex))
//	NextIsMessage = Modify(tlwire.LowEncoder{}.AppendSemantic(nil, WireMessage))
)

const (
	WireLabel = tlwire.SemanticTlogBase + iota
	WireID
	WireMessage
	WireEventKind
	WireLogLevel

	_
	_
	_
	_
	_

	SemanticUserBase
)

var (
	e tlwire.Encoder
	d tlwire.Decoder
)

func AppendKVs(b []byte, kvs []interface{}) []byte {
	return appendKVs0(b, kvs)
}

func NextIs(tag int) Modify {
	return Modify(tlwire.LowEncoder{}.AppendSemantic(nil, tag))
}

func Special(tag byte) RawMessage {
	return RawMessage(tlwire.LowEncoder{}.AppendSpecial(nil, tag))
}

//go:linkname appendKVs0 github.com/nikandfor/tlog.appendKVs
//go:noescape
func appendKVs0(b []byte, kvs []interface{}) []byte

func appendKVs(b []byte, kvs []interface{}) []byte {
	for i := 0; i < len(kvs); {
		var k string

		switch el := kvs[i].(type) {
		case string:
			k = el

			if k == KeyAuto {
				k = autoKey(kvs[i:])
			}

			i++
		case RawMessage:
			b = append(b, el...)
			i++
			continue
		default:
			k = "MISSING_KEY"
		}

		b = e.AppendString(b, k)

	value:
		if i == len(kvs) {
			b = append(b, tlwire.Special|tlwire.Undefined)
			break
		}

		switch v := kvs[i].(type) {
		case string:
			b = e.AppendString(b, v)
		case int:
			b = e.AppendInt(b, v)
		case RawMessage:
			b = append(b, v...)
		case Modify:
			b = append(b, v...)
			i++

			goto value
		case FormatNext:
			i++
			if i == len(kvs) {
				b = append(b, tlwire.Special|tlwire.Undefined)
				break
			}

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
	//	case Message:
	//		k = KeyMessage
	case ID:
		k = KeySpan
	case LogLevel:
		k = KeyLogLevel
	case EventKind:
		k = KeyEventKind
	case loc.PC:
		k = KeyCaller
	case loc.PCs:
		k = KeyCaller
	default:
		k = "UNSUPPORTED_AUTO_KEY"
	}

	return
}

func (id ID) TlogAppend(b []byte) []byte {
	b = append(b, tlwire.Semantic|WireID)
	return e.AppendBytes(b, id[:])
}

func (id *ID) TlogParse(p []byte, i int) int {
	if p[i] != tlwire.Semantic|WireID {
		panic("not an id")
	}

	i++

	if p[i] != tlwire.Bytes|16 {
		panic("not an id")
	}

	i++

	i += copy((*id)[:], p[i:])

	return i
}

func (ek EventKind) TlogAppend(b []byte) []byte {
	b = append(b, tlwire.Semantic|WireEventKind)
	return e.AppendString(b, string(ek))
}

func (ek *EventKind) TlogParse(p []byte, i int) int {
	if p[i] != tlwire.Semantic|WireEventKind {
		panic("not an event type")
	}

	i++

	v, i := d.Bytes(p, i)

	r, w := utf8.DecodeRune(v)
	if w == utf8.RuneError || w != len(v) {
		panic("bad rune")
	}

	*ek = EventKind(r)

	return i
}

func (l LogLevel) TlogAppend(b []byte) []byte {
	b = append(b, tlwire.Semantic|WireLogLevel)
	return e.AppendInt(b, int(l))
}

func (l *LogLevel) TlogParse(p []byte, i int) int {
	if p[i] != tlwire.Semantic|WireLogLevel {
		panic("not a log level")
	}

	i++

	v, i := d.Signed(p, i)

	*l = LogLevel(v)

	return i
}

func (r RawMessage) TlogAppend(b []byte) []byte {
	return append(b, r...)
}

func (r *RawMessage) TlogParse(p []byte, st int) (i int) {
	i = d.Skip(p, st)
	*r = append((*r)[:0], p[st:i]...)
	return i
}
