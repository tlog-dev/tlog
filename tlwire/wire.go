package tlwire

// Basic types.
const (
	Int = iota << 5
	Neg
	Bytes
	String
	Array
	Map
	Semantic
	Special

	TagMask    = 0b111_00000
	TagDetMask = 0b000_11111
)

// Len.
const (
	Len1 = 24 + iota
	Len2
	Len4
	Len8
	_
	_
	_
	LenBreak = Break
)

// Specials.
const (
	False = 20 + iota
	True
	Nil
	Undefined

	Float8
	Float16
	Float32
	Float64
	_
	_
	_
	Break

	None    = 19 // used to preserve padding
	Hidden  = 18 // passwords, etc. when you want to preserve the key
	SelfRef = 17 // self reference
)

// Semantics.
const (
	Meta = iota
	Error
	Time
	Duration
	Big

	Caller
	_
	Hex
	_
	_

	SemanticTlogBase
)

func init() {
	if Break != TagDetMask {
		panic(Break)
	}
}
