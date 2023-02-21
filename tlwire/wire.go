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
	_ = 1<<5 - iota

	Break

	_
	_
	_

	Float64
	Float32
	Float16
	Float8

	Undefined
	Nil
	True
	False // 20
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
	Represent // ?

	SemanticTlogBase
)
