package tlog

import (
	"unicode/utf8"
	_ "unsafe"

	"github.com/nikandfor/hacked/low"
	"tlog.app/go/loc"

	"tlog.app/go/tlog/tlwire"
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
	Hidden        = RawMessage{tlwire.Special | tlwire.Hidden}
	None          = RawMessage{tlwire.Special | tlwire.None}
	Nil           = RawMessage{tlwire.Special | tlwire.Nil}
	Break         = RawMessage{tlwire.Special | tlwire.Break}
	NextAsHex     = Modify{tlwire.Semantic | tlwire.Hex}
	NextAsMessage = Modify{tlwire.Semantic | WireMessage}
	NextAsType    = FormatNext("%T")
)

// Wire semantic tags.
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

	SemanticCommunityBase = tlwire.SemanticTlogBase + 10
)

func AppendLabels(e tlwire.Encoder, b []byte, kvs []interface{}) []byte {
	const tag = tlwire.Semantic | WireLabel

	var d tlwire.LowDecoder

	w := len(b)
	b = append(b, low.Spaces[:len(kvs)/2+1]...)
	r := len(b)

	b = AppendKVs(e, b, kvs)

	for r < len(b) {
		end := d.Skip(b, r)

		w += copy(b[w:], b[r:end])
		r = end

		end = d.Skip(b, r)

		if b[r] != tag {
			b[w] = tag
			w++
		}

		w += copy(b[w:], b[r:end])
		r = end
	}

	return b[:w]
}

func AppendKVs(e tlwire.Encoder, b []byte, kvs []interface{}) []byte {
	return appendKVs0(e, b, kvs)
}

func NextIs(semantic int) Modify {
	return Modify(tlwire.LowEncoder{}.AppendTag(nil, tlwire.Semantic, semantic))
}

func RawTag(tag byte, sub int) RawMessage {
	return RawMessage(tlwire.LowEncoder{}.AppendTag(nil, tag, sub))
}

func Special(value int) RawMessage {
	return RawMessage(tlwire.LowEncoder{}.AppendTag(nil, tlwire.Special, value))
}

//go:linkname appendKVs0 tlog.app/go/tlog.appendKVs
//go:noescape
func appendKVs0(e tlwire.Encoder, b []byte, kvs []interface{}) []byte

func init() { // prevent deadcode warnings
	appendKVs(tlwire.Encoder{}, nil, nil)
}

func appendKVs(e tlwire.Encoder, b []byte, kvs []interface{}) []byte {
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
		case tlwire.TlogAppender:
			b = el.TlogAppend(b)
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

	if m, ok := kvs[1].(Modify); ok && string(m) == string(NextAsMessage) {
		return KeyMessage
	}

	switch kvs[1].(type) {
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

func (ek EventKind) String() string {
	return string(ek)
}

func (id ID) TlogAppend(b []byte) []byte {
	var e tlwire.LowEncoder
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
	var e tlwire.LowEncoder
	b = append(b, tlwire.Semantic|WireEventKind)
	return e.AppendString(b, string(ek))
}

func (ek *EventKind) TlogParse(p []byte, i int) int {
	var d tlwire.LowDecoder

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
	var e tlwire.LowEncoder
	b = append(b, tlwire.Semantic|WireLogLevel)
	return e.AppendInt(b, int(l))
}

func (l *LogLevel) TlogParse(p []byte, i int) int {
	var d tlwire.LowDecoder

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
	var d tlwire.LowDecoder
	i = d.Skip(p, st)
	*r = append((*r)[:0], p[st:i]...)
	return i
}
