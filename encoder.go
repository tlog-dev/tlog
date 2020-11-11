package tlog

import (
	"math"
	_ "unsafe"
)

/*
	Format

	It's intended to be compact, fast encodable and resources effective.

	Stream consists of Records. Record consists of fields.

	Field is some kind of tag-len-value but optimized for compactness.

	Record is terminated by `0` tag.

	Record can be skipped only field-by-field until `0` got.

	Tags

	0 - end of record
	r - Record Type
	s - Span ID
	t - Wall Time
	i - Log Level (importance)
	l - Location ID (Program Counter, maybe compressed)
	m - Message (or Name)
	f - additional user defined Field data
	v - Observation
	e - Predefined type

	Encoding

	0 is 0x00 byte
	i is 0b0001_xxxx where xxxx is int4 log level
	s is 0b0010_xxxx where xxxx is ID len - 1
	v is 0b0011_xxxx where xxxx is int value or size of float value
	v is 0b0100_xxxx where xxxx is uint value or size of uint value
	v is 0b0101_xxxx where xxxx is negative int
	e is 0b0110_xxxx where xxxx is value if v < 1<<4-4

	m is 0b10xx_xxxx where xx_xxxx is str_len
		followed by string
	f is 0b11xx_xxxx where xx_xxxx is key_len
		followed by key
		followed by value

	t is 0b0000_0001 followed by 8 bytes unix nano time
	r is 0b0000_0010 followed by Type byte

	Value

	value is inspired by CBOR

	0bxxxy_yyyy where xxx is base type, y_yyyy is additional info

	Base Types

	0b000 - positive int
	0b001 - negative int +1 (0 means -1, 1 = -2...)
	0b010 - byte string
	0b011 - text string
	0b100 - array
	0b101 - map with keys in 0b11xx_xxxx format
	0b110 - predefined type follows
	0b111 - specials

	Predefined Types

	0 - general object
	1 - location
	2 - duration
	3 - parent ID
	4 - func name
	5 - file
	6 - line

*/

// record tags
const (
	rEnd  = 0
	rTime = 1
	rType = 2
	_

	rLevel  = 0x10
	rSpan   = 0x20
	rValFlt = 0x30
	rValInt = 0x40
	rValNeg = 0x50
	rExt    = 0x60
	_

	rMessage = 0b1000_0000
	rField   = 0b1100_0000
)

// base types
const (
	tInt = iota << 5
	tNegInt
	tBytes
	tString
	tArray
	tMap
	tPredef
	tSpec

	tFlt4 = tSpec | 26
	tFlt8 = tSpec | 27
)

// extension types
const (
	eStruct = iota
	eLocation
	eDuration
	eParent
	eFuncName
	eFile
	eLine
)

type (
	encoder struct {
		ls map[PC]int

		b []byte
	}

	Format struct {
		F string
		V interface{}
	}
)

func (e *encoder) reset() {
	e.b = e.b[:0]
}

func (e *encoder) eor() {
	e.b = append(e.b, rEnd)
}

func (e *encoder) parent(id ID) {
	e.b = append(e.b, rExt|eParent, tBytes|byte(len(id)))
	e.b = append(e.b, id[:]...)
}

func (e *encoder) duration(d int64) {
	e.b = append(e.b, rExt|eDuration)
	e.b = e.appendInt(e.b, d)
}

func (e *encoder) predef(t int64) {
	const tag = rExt

	switch {
	case t < 1<<4-4:
		e.b = append(e.b, tag|byte(t))
	case t <= 0xff:
		e.b = append(e.b, tag|1<<4-4, byte(t))
	case t <= 0xffff:
		e.b = append(e.b, tag|1<<4-3, byte(t>>8), byte(t))
	case t <= 0xffff_ffff:
		e.b = append(e.b, tag|1<<4-2, byte(t>>24), byte(t>>16), byte(t>>8), byte(t))
	default:
		e.b = append(e.b, tag|1<<4-1, byte(t>>56), byte(t>>48), byte(t>>40), byte(t>>32), byte(t>>24), byte(t>>16), byte(t>>8), byte(t))
	}
}

func (e *encoder) loglevel(lv Level) {
	e.b = append(e.b, rLevel|byte(lv&0xf))
}

func (e *encoder) rectype(tp Type) {
	e.b = append(e.b, rType, byte(tp))
}

func (e *encoder) id(id ID) {
	e.b = append(e.b, rSpan)
	e.b = append(e.b, id[:]...)
}

func (e *encoder) timestamp(ts int64) {
	e.b = append(e.b,
		rTime, // tag
		byte(ts>>56),
		byte(ts>>48),
		byte(ts>>40),
		byte(ts>>32),
		byte(ts>>24),
		byte(ts>>16),
		byte(ts>>8),
		byte(ts),
	)
}

func (e *encoder) valueFloat(v float64) {
	if q := int(v); q >= 0 && q <= 1<<4-2 && float64(q) == v {
		e.b = append(e.b, rValInt|byte(q))
		return
	}

	if q := float32(v); float64(q) == v {
		bits := math.Float32bits(q)

		e.b = append(e.b, rValFlt|1<<4-2, byte(bits>>24), byte(bits>>16), byte(bits>>8), byte(bits))

		return
	}

	bits := math.Float64bits(v)

	e.b = append(e.b, rValFlt|1<<4-1, byte(bits>>56), byte(bits>>48), byte(bits>>40), byte(bits>>32), byte(bits>>24), byte(bits>>16), byte(bits>>8), byte(bits))
}

func (e *encoder) valueInt(v int64) {
	var q uint64
	tag := byte(tInt)
	if v >= 0 {
		q = uint64(v)
	} else {
		tag = tNegInt
		q = uint64(-v) + 1
	}

	switch {
	case q < 1<<4-4:
		e.b = append(e.b, tag|byte(q))
	case q <= 0xff:
		e.b = append(e.b, tag|1<<4-4, byte(q))
	case q <= 0xffff:
		e.b = append(e.b, tag|1<<4-3, byte(q>>8), byte(q))
	case q <= 0xffff_ffff:
		e.b = append(e.b, tag|1<<4-2, byte(q>>24), byte(q>>16), byte(q>>8), byte(q))
	default:
		e.b = append(e.b, tag|1<<4-1, byte(q>>56), byte(q>>48), byte(q>>40), byte(q>>32), byte(q>>24), byte(q>>16), byte(q>>8), byte(q))
	}

	return
}

func (e *encoder) caller(pc PC) {
	v, ok := e.ls[pc]
	if !ok {
		v = len(e.ls) + 1
		e.ls[pc] = v
	}

	e.b = append(e.b, rExt|eLocation)
	e.b = e.appendInt(e.b, int64(v))

	if ok {
		return
	}

	name, file, line := pc.NameFileLine()

	e.b = append(e.b, rExt|eFuncName)
	e.b = e.appendString(e.b, tString, name)

	e.b = append(e.b, rExt|eFile)
	e.b = e.appendString(e.b, tString, file)

	e.b = append(e.b, rExt|eLine)
	e.b = e.appendInt(e.b, int64(line))
}

func (e *encoder) message(f string, args []interface{}) {
	e.b = append(e.b,
		rMessage, // tag
	)

	st := len(e.b)

	switch {
	case len(args) == 0:
		e.b = append(e.b, f...)
	case f == "":
		e.b = AppendPrintln(e.b, args...)
	default:
		e.b = AppendPrintf(e.b, f, args...)
	}

	l := len(e.b) - st
	l--

	if l < 1<<6-4 {
		e.b[st-1] |= byte(l)

		return
	}

	e.b = e.insertLen(e.b, st, l)
}

func (e *encoder) insertLen(b []byte, st, l int) []byte {
	var sz int

	switch {
	case l <= 0xff:
		b[st-1] |= 1<<6 - 4
		sz = 1
	case l <= 0xffff:
		b[st-1] |= 1<<6 - 3
		sz = 2
	case l <= 0xffff_ffff:
		b[st-1] |= 1<<6 - 2
		sz = 4
	default:
		b[st-1] |= 1<<6 - 1
		sz = 8
	}

	b = append(b, []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0}[:sz]...)
	copy(b[st+sz:], b[st:])

	for i := st + sz; i > st; i-- {
		b[i] = byte(l)
		l >>= 8
	}

	return b
}

func (e *encoder) kvs(kvs ...interface{}) {
	i := 0
	for i < len(kvs) {
		k := kvs[i].(string)
		i++

		if len(k) > 1<<6-4 {
			panic(len(k))
		}

		e.b = append(e.b, rField|byte(len(k)))
		e.b = append(e.b, k...)

		e.b = e.appendValue(e.b, kvs[i])
		i++
	}
}

// force noescape
// //go:linkname appendValue github.com/nikandfor/tlog.(*encoder).appendValue1
// //go:noescape
// func appendValue(e *encoder, b []byte, v interface{}) []byte

func (e *encoder) appendValue(b []byte, v interface{}) []byte {
	switch v := v.(type) {
	case string:
		b = e.appendString(b, tString, v)
	case []byte:
		b = e.appendString(b, tBytes, bytesToString(v))
	case int:
		b = e.appendInt(b, int64(v))
	case int64:
		b = e.appendInt(b, int64(v))
	case int32:
		b = e.appendInt(b, int64(v))
	case int16:
		b = e.appendInt(b, int64(v))
	case int8:
		b = e.appendInt(b, int64(v))
	case uint:
		b = e.appendUint(b, uint64(v))
	case uint64:
		b = e.appendUint(b, uint64(v))
	case uint32:
		b = e.appendUint(b, uint64(v))
	case uint16:
		b = e.appendUint(b, uint64(v))
	case uint8:
		b = e.appendUint(b, uint64(v))
	case float64:
		b = e.appendFloat(b, v)
	case float32:
		b = e.appendFloat(b, float64(v))
	case Format:
		b = append(b, tString)
		st := len(b)
		b = AppendPrintf(b, v.F, v.V)

		l := len(b) - st
		if l < 1<<5-4 {
			b[st-1] |= byte(l)
		} else {
			b = e.insertLen(b, st, l)
		}
	default:
		panic("unsupported value type")
	}

	return b
}

func (e *encoder) appendString(b []byte, tag byte, v string) []byte {
	b = append(b, tag)
	st := len(b)
	b = append(b, v...)

	if len(v) < 1<<5-4 {
		b[st-1] |= byte(len(v))
	} else {
		b = e.insertLen(b, st, len(v))
	}

	return b
}

func (e *encoder) appendInt(b []byte, v int64) []byte {
	tag := byte(tInt)
	if v < 0 {
		tag = tNegInt
		v = -v + 1
	}

	switch {
	case v < 1<<5-4:
		b = append(b, tag|byte(v))
	case v <= 0xff:
		b = append(b, tag|1<<5-4, byte(v))
	case v <= 0xffff:
		b = append(b, tag|1<<5-3, byte(v>>8), byte(v))
	case v <= 0xffff_ffff:
		b = append(b, tag|1<<5-2, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	default:
		b = append(b, tag|1<<5-1, byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32), byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	}

	return b
}

func (e *encoder) appendUint(b []byte, v uint64) []byte {
	switch {
	case v < 1<<5-4:
		b = append(b, byte(v))
	case v <= 0xff:
		b = append(b, 1<<5-4, byte(v))
	case v <= 0xffff:
		b = append(b, 1<<5-3, byte(v>>8), byte(v))
	case v <= 0xffff_ffff:
		b = append(b, 1<<5-2, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	default:
		b = append(b, 1<<5-1, byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32), byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	}

	return b
}

func (e *encoder) appendFloat(b []byte, v float64) []byte {
	if q := int64(v); float64(q) == v && (q <= 0xffff_ffff && q >= -0xffff_ffff) {
		return e.appendInt(b, q)
	}

	if q := float32(v); float64(q) == v {
		bits := math.Float32bits(q)

		return append(b, tFlt4, byte(bits>>24), byte(bits>>16), byte(bits>>8), byte(bits))
	}

	bits := math.Float64bits(v)

	return append(b, tFlt8, byte(bits>>56), byte(bits>>48), byte(bits>>40), byte(bits>>32), byte(bits>>24), byte(bits>>16), byte(bits>>8), byte(bits))
}
