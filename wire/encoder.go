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
	Null
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
	return e.AppendTag(b, Map, int64(l))
}

func (e *Encoder) AppendArray(b []byte, l int) []byte {
	return e.AppendTag(b, Array, int64(l))
}

func (e *Encoder) AppendBreak(b []byte) []byte {
	return append(b, Special|Break)
}

func (e *Encoder) AppendKey(b []byte, k string) []byte {
	return e.AppendString(b, String, k)
}

func (e *Encoder) AppendKeyString(b []byte, k, v string) []byte {
	b = e.AppendString(b, String, k)
	b = e.AppendString(b, String, v)
	return b
}

func (e *Encoder) AppendKeyBytes(b []byte, k string, v []byte) []byte {
	b = e.AppendString(b, String, k)
	b = e.AppendStringBytes(b, Bytes, v)
	return b
}

func (e *Encoder) AppendKeyInt(b []byte, k string, v int64) []byte {
	b = e.AppendString(b, String, k)
	b = e.AppendSigned(b, v)
	return b
}

func (e *Encoder) AppendKeyUint(b []byte, k string, v uint64) []byte {
	b = e.AppendString(b, String, k)
	b = e.AppendInt(b, Int, v)
	return b
}

func (e *Encoder) AppendKeyValue(b []byte, k string, v interface{}) []byte {
	b = e.AppendString(b, String, k)
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

	_ = e.AppendTag(b[:st], String, int64(l))

	return b
}

func (e *Encoder) AppendError(b []byte, err error) []byte {
	b = append(b, Semantic|Error)

	if err == nil {
		return append(b, Special|Null)
	}

	return e.AppendString(b, String, err.Error())
}

func (e *Encoder) AppendTime(b []byte, t time.Time) []byte {
	b = append(b, Semantic|Time)
	b = e.AppendInt(b, Int, uint64(t.UnixNano()))
	return b
}

func (e *Encoder) AppendTimestamp(b []byte, t int64) []byte {
	b = append(b, Semantic|Time)
	b = e.AppendInt(b, Int, uint64(t))
	return b
}

func (e *Encoder) AppendDuration(b []byte, d time.Duration) []byte {
	b = append(b, Semantic|Duration)
	b = e.AppendInt(b, Int, uint64(d.Nanoseconds()))
	return b
}

func (e *Encoder) AppendBigInt(b []byte, x *big.Int) []byte {
	b = append(b, Semantic|Big)

	if x == nil {
		return append(b, Special|Null)
	}

	b = e.AppendStringBytes(b, Bytes, x.Bytes())

	return b
}

func (e *Encoder) AppendBigRat(b []byte, x *big.Rat) []byte {
	b = append(b, Semantic|Big)

	if x == nil {
		return append(b, Special|Null)
	}

	b = append(b, Array|2)

	b = e.AppendStringBytes(b, Bytes, x.Num().Bytes())
	b = e.AppendStringBytes(b, Bytes, x.Denom().Bytes())

	return b
}

func (e *Encoder) AppendBigFloat(b []byte, x *big.Float) []byte {
	b = append(b, Semantic|Big)

	if x == nil {
		return append(b, Special|Null)
	}

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

func (e *LowEncoder) AppendString(b []byte, tag byte, s string) []byte {
	b = e.AppendTag(b, tag, int64(len(s)))
	return append(b, s...)
}

func (e *LowEncoder) AppendStringBytes(b []byte, tag byte, s []byte) []byte {
	b = e.AppendTag(b, tag, int64(len(s)))
	return append(b, s...)
}

func (e *LowEncoder) AppendTag(b []byte, tag byte, v int64) []byte {
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

func (e *LowEncoder) AppendSigned(b []byte, v int64) []byte {
	if v < 0 {
		return e.AppendInt(b, Neg, uint64(-v))
	}

	return e.AppendInt(b, Int, uint64(v))
}

func (e *LowEncoder) AppendInt(b []byte, tag byte, v uint64) []byte {
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
