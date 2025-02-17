package tlwire

import "nikand.dev/go/cbor"

type (
	Tag = cbor.Tag

	LowEncoder struct {
		cbor.Emitter
	}
	LowDecoder struct {
		cbor.Iterator
	}
)

// Major types.
const (
	Int    = cbor.Int
	Neg    = cbor.Neg
	Bytes  = cbor.Bytes
	String = cbor.String
	Array  = cbor.Array
	Map    = cbor.Map
	Label  = cbor.Label
	Simple = cbor.Simple

	Semantic = Label
	Special  = Simple

	TagMask    = cbor.TagMask
	SubMask    = cbor.SubMask
	TagDetMask = SubMask
)

// Len.
const (
	Len1 = cbor.Len1
	Len2 = cbor.Len2
	Len4 = cbor.Len4
	Len8 = cbor.Len8

	LenBreak = Break
)

// Specials.
const (
	False     = cbor.False
	True      = cbor.True
	Nil       = cbor.Null
	Undefined = cbor.Undefined

	Float8  = cbor.Float8
	Float16 = cbor.Float16
	Float32 = cbor.Float32
	Float64 = cbor.Float64

	Break = cbor.Break

	None = cbor.None

	SelfRef = 10 // self reference
	Hidden  = 11 // passwords, etc. when you want to preserve the key
)

// Semantics.
const (
	Meta = iota
	Error
	Time
	Duration
	Big

	Caller
	NetAddr
	Hex
	_
	Embedding

	LabelTlogBase    = 10
	SemanticTlogBase = LabelTlogBase
)

// Meta.
const (
	MetaMagic = iota
	MetaVer

	MetaTlogBase = 8
)

const Magic = "\xc0\x64tlog"

func (e LowEncoder) AppendSemantic(b []byte, l int) []byte { return e.AppendLabel(b, l) }
func (e *Encoder) AppendSemantic(b []byte, l int) []byte   { return e.AppendLabel(b, l) }
