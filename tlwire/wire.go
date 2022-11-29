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
	None = 19 + iota
	False
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
	Break = 1<<5 - 1
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
