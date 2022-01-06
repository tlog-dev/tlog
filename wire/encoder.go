package wire

import (
	"encoding/binary"
	"math"
	"math/big"
	"time"

	"github.com/nikandfor/tlog/low"
)

type (
	Encoder struct {
		LowEncoder
	}

	LowEncoder struct{}
)

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

	SemanticExtBase
)

func (e *Encoder) AppendMap(b []byte, l int) []byte {
	return e.AppendTag(b, Map, l)
}

func (e *Encoder) AppendArray(b []byte, l int) []byte {
	return e.AppendTag(b, Array, l)
}

func (e *Encoder) AppendBreak(b []byte) []byte {
	return append(b, Special|Break)
}

func (e *Encoder) AppendNil(b []byte) []byte {
	return append(b, Special|Nil)
}

func (e *Encoder) AppendKey(b []byte, k string) []byte {
	return e.AppendTagString(b, String, k)
}

func (e *Encoder) AppendKeyString(b []byte, k, v string) []byte {
	b = e.AppendString(b, k)
	b = e.AppendString(b, v)
	return b
}

func (e *Encoder) AppendKeyBytes(b []byte, k string, v []byte) []byte {
	b = e.AppendString(b, k)
	b = e.AppendBytes(b, v)
	return b
}

func (e *Encoder) AppendKeyInt(b []byte, k string, v int) []byte {
	b = e.AppendString(b, k)
	b = e.AppendInt(b, v)
	return b
}

func (e *Encoder) AppendKeyUint(b []byte, k string, v uint) []byte {
	b = e.AppendString(b, k)
	b = e.AppendUint(b, v)
	return b
}

func (e *Encoder) AppendKeyInt64(b []byte, k string, v int64) []byte {
	b = e.AppendString(b, k)
	b = e.AppendInt64(b, v)
	return b
}

func (e *Encoder) AppendKeyUint64(b []byte, k string, v uint64) []byte {
	b = e.AppendString(b, k)
	b = e.AppendUint64(b, v)
	return b
}

func (e *Encoder) AppendKeyValue(b []byte, k string, v interface{}) []byte {
	b = e.AppendString(b, k)
	b = e.AppendValue(b, v)
	return b
}

func (e *Encoder) AppendFormat(b []byte, fmt string, args ...interface{}) []byte {
	st := len(b)

	b = append(b, String)

	if fmt == "" {
		b = low.AppendPrint(b, args...)
	} else {
		b = low.AppendPrintf(b, fmt, args...)
	}

	l := len(b) - st - 1

	if l < Len1 {
		b[st] |= byte(l)

		return b
	}

	var sz int

	switch {
	case l <= 0xff:
		sz = 1
	case l <= 0xffff:
		sz = 2
	case l <= 0xffff_ffff:
		sz = 4
	default:
		sz = 8
	}

	b = append(b, []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0}[:sz]...)
	copy(b[st+sz:], b[st:])

	_ = e.AppendTag(b[:st], String, l)

	return b
}

func (e *Encoder) AppendError(b []byte, err error) []byte {
	b = append(b, Semantic|Error)

	if err == nil {
		return append(b, Special|Nil)
	}

	return e.AppendString(b, err.Error())
}

func (e *Encoder) AppendTime(b []byte, t time.Time) []byte {
	b = append(b, Semantic|Time)
	b = e.AppendTagInt(b, Int, uint64(t.UnixNano()))
	return b
}

func (e *Encoder) AppendTimestamp(b []byte, t int64) []byte {
	b = append(b, Semantic|Time)
	b = e.AppendInt64(b, t)
	return b
}

func (e *Encoder) AppendDuration(b []byte, d time.Duration) []byte {
	b = append(b, Semantic|Duration)
	b = e.AppendInt64(b, d.Nanoseconds())
	return b
}

func (e *Encoder) AppendBigInt(b []byte, x *big.Int) []byte {
	b = append(b, Semantic|Big)

	if x == nil {
		return append(b, Special|Nil)
	}

	if false {
		return e.AppendBytes(b, x.Bytes())
	}

	return e.appendBigInt(b, x)
}

func (e *Encoder) appendBigInt(b []byte, x *big.Int) []byte {
	if x.IsUint64() {
		return e.AppendUint64(b, x.Uint64())
	}

	if x.IsInt64() {
		return e.AppendInt64(b, x.Int64())
	}

	b = append(b, Semantic|byte(BigInt))
	b = append(b, String|0)

	st := len(b)
	b = x.Append(b, 10)
	l := len(b) - st

	b = e.InsertLen(b, st, l)

	return b
}

func (e *Encoder) AppendBigRat(b []byte, x *big.Rat) []byte {
	b = append(b, Semantic|Big)

	if x == nil {
		return append(b, Special|Nil)
	}

	if false {
		b = append(b, Array|2)

		b = e.AppendBytes(b, x.Num().Bytes())
		b = e.AppendBytes(b, x.Denom().Bytes())
	}

	if false && x.IsInt() {
		return e.appendBigInt(b, x.Num())
	}

	b = append(b, Array|2)

	b = e.appendBigInt(b, x.Num())
	b = e.appendBigInt(b, x.Denom())

	return b
}

func (e *Encoder) AppendBigFloat(b []byte, x *big.Float) []byte {
	b = append(b, Semantic|Big)

	if x == nil {
		return append(b, Special|Nil)
	}

	if v, a := x.Float64(); a == big.Exact {
		return e.AppendFloat(b, v)
	}

	b = append(b, Semantic|byte(BigFloat))
	b = append(b, String|0)

	st := len(b)
	b = x.Append(b, 'g', -1)
	l := len(b) - st

	b = e.InsertLen(b, st, l)

	return b
}

func (e *Encoder) InsertLen(b []byte, st, l int) []byte {
	if l < 0 {
		panic(l)
	}

	if l < Len1 {
		b[st-1] = b[st-1]&TagMask | byte(l)

		return b
	}

	m := 0
	switch {
	case l < 0xff:
		m = 1
	case l < 0xffff:
		m = 2
	case l < 0xffff_ffff:
		m = 4
	default:
		m = 8
	}

	b = append(b, "        "[:m]...)

	copy(b[st+m:], b[st:])

	b[st-1] = b[st-1] & TagMask

	switch {
	case l < 0xff:
		b[st-1] |= Len1

		b[st] = byte(l)
	case l < 0xffff:
		b[st-1] |= Len2
		binary.BigEndian.PutUint16(b[st:], uint16(l))
	case l < 0xffff_ffff:
		b[st-1] |= Len4
		binary.BigEndian.PutUint32(b[st:], uint32(l))
	default:
		b[st-1] |= Len8
		binary.BigEndian.PutUint64(b[st:], uint64(l))
	}

	return b
}

func (e *LowEncoder) AppendString(b []byte, s string) []byte {
	b = e.AppendTag(b, String, len(s))
	return append(b, s...)
}

func (e *LowEncoder) AppendBytes(b, s []byte) []byte {
	b = e.AppendTag(b, Bytes, len(s))
	return append(b, s...)
}

func (e *LowEncoder) AppendTagString(b []byte, tag byte, s string) []byte {
	b = e.AppendTag(b, tag, len(s))
	return append(b, s...)
}

func (e *LowEncoder) AppendTagBytes(b []byte, tag byte, s []byte) []byte {
	b = e.AppendTag(b, tag, len(s))
	return append(b, s...)
}

func (e *LowEncoder) AppendTag(b []byte, tag byte, v int) []byte {
	switch {
	case v == -1:
		return append(b, tag|LenBreak)
	case v < Len1:
		return append(b, tag|byte(v))
	case v <= 0xff:
		return append(b, tag|Len1, byte(v))
	case v <= 0xffff:
		return append(b, tag|Len2, byte(v>>8), byte(v))
	case v <= 0xffff_ffff:
		return append(b, tag|Len4, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	default:
		return append(b, tag|Len8, byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32), byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	}
}

func (e *LowEncoder) AppendTag64(b []byte, tag byte, v int64) []byte {
	switch {
	case v == -1:
		return append(b, tag|LenBreak)
	case v < Len1:
		return append(b, tag|byte(v))
	case v <= 0xff:
		return append(b, tag|Len1, byte(v))
	case v <= 0xffff:
		return append(b, tag|Len2, byte(v>>8), byte(v))
	case v <= 0xffff_ffff:
		return append(b, tag|Len4, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	default:
		return append(b, tag|Len8, byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32), byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	}
}

func (e *LowEncoder) AppendSemantic(b []byte, s int) []byte {
	return e.AppendTag(b, Semantic, s)
}

func (e *LowEncoder) AppendInt(b []byte, v int) []byte {
	if v < 0 {
		return e.AppendTagInt(b, Neg, uint64(-v))
	}

	return e.AppendTagInt(b, Int, uint64(v))
}

func (e *LowEncoder) AppendUint(b []byte, v uint) []byte {
	return e.AppendTagInt(b, Int, uint64(v))
}

func (e *LowEncoder) AppendInt64(b []byte, v int64) []byte {
	if v < 0 {
		return e.AppendTagInt(b, Neg, uint64(-v))
	}

	return e.AppendTagInt(b, Int, uint64(v))
}

func (e *LowEncoder) AppendUint64(b []byte, v uint64) []byte {
	return e.AppendTagInt(b, Int, v)
}

func (e *LowEncoder) AppendTagInt(b []byte, tag byte, v uint64) []byte {
	switch {
	case v < Len1:
		return append(b, tag|byte(v))
	case v <= 0xff:
		return append(b, tag|Len1, byte(v))
	case v <= 0xffff:
		return append(b, tag|Len2, byte(v>>8), byte(v))
	case v <= 0xffff_ffff:
		return append(b, tag|Len4, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	default:
		return append(b, tag|Len8, byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32), byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	}
}

func (e *LowEncoder) AppendFloat(b []byte, v float64) []byte {
	if q := int8(v); float64(q) == v {
		return append(b, Special|Float8, byte(q))
	}

	if q := float32(v); float64(q) == v {
		r := math.Float32bits(q)

		return append(b, Special|Float32, byte(r>>24), byte(r>>16), byte(r>>8), byte(r))
	}

	r := math.Float64bits(v)

	return append(b, Special|Float64, byte(r>>56), byte(r>>48), byte(r>>40), byte(r>>32), byte(r>>24), byte(r>>16), byte(r>>8), byte(r))
}

func (e *LowEncoder) AppendSpecial(b []byte, x byte) []byte {
	return append(b, Special|x)
}

func (e *LowEncoder) AppendBreak(b []byte) []byte {
	return append(b, Special|Break)
}
