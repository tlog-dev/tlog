package tlwire

import (
	"math"
	"net/netip"
	"time"

	"nikand.dev/go/hacked/hfmt"
	"nikand.dev/go/hacked/htime"
)

type (
	Encoder struct {
		LowEncoder

		custom encoders
	}

	LowEncoder struct{}
)

func (e *Encoder) AppendKey(b []byte, key string) []byte {
	b = e.AppendTag(b, String, len(key))
	return append(b, key...)
}

func (e *Encoder) AppendKeyString(b []byte, k, v string) []byte {
	b = e.AppendTag(b, String, len(k))
	b = append(b, k...)

	b = e.AppendTag(b, String, len(v))
	b = append(b, v...)

	return b
}

func (e *Encoder) AppendKeyInt(b []byte, k string, v int) []byte {
	b = e.AppendTag(b, String, len(k))
	b = append(b, k...)
	return e.AppendInt(b, v)
}

func (e *Encoder) AppendKeyUint(b []byte, k string, v uint) []byte {
	b = e.AppendTag(b, String, len(k))
	b = append(b, k...)
	return e.AppendTag64(b, Int, uint64(v))
}

func (e *Encoder) AppendKeyInt64(b []byte, k string, v int64) []byte {
	b = e.AppendTag(b, String, len(k))
	b = append(b, k...)

	if v < 0 {
		return e.AppendTag64(b, Neg, uint64(-v)+1)
	}

	return e.AppendTag64(b, Int, uint64(v))
}

func (e *Encoder) AppendKeyUint64(b []byte, k string, v uint64) []byte {
	b = e.AppendTag(b, String, len(k))
	b = append(b, k...)
	return e.AppendTag64(b, Int, v)
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
	return e.AppendInt64(b, t.UnixNano())
}

func (e *Encoder) AppendTimeTZ(b []byte, t time.Time) []byte {
	b = append(b, Semantic|Time)
	b = append(b, Map|2)

	b = e.AppendKeyInt64(b, "t", t.UnixNano())

	b = e.AppendKey(b, "z")
	b = append(b, Array|2)

	n, off := t.Zone()
	b = e.AppendString(b, n)
	b = e.AppendInt(b, off)

	return b
}

func (e *Encoder) AppendTimeMonoTZ(b []byte, t time.Time) []byte {
	b = append(b, Semantic|Time)

	mono := htime.MonotonicOf(t)
	fields := 2

	if mono != 0 {
		fields++
	}

	b = append(b, Map|byte(fields))

	b = e.AppendKeyInt64(b, "t", t.UnixNano())

	if mono != 0 {
		b = e.AppendKeyInt64(b, "m", mono)
	}

	b = e.AppendKey(b, "z")
	b = append(b, Array|2)

	n, off := t.Zone()
	b = e.AppendString(b, n)
	b = e.AppendInt(b, off)

	return b
}

func (e *Encoder) AppendTimestamp(b []byte, t int64) []byte {
	b = append(b, Semantic|Time)
	return e.AppendInt64(b, t)
}

func (e *Encoder) AppendDuration(b []byte, d time.Duration) []byte {
	b = append(b, Semantic|Duration)
	return e.AppendInt64(b, d.Nanoseconds())
}

func (e *Encoder) AppendAddr(b []byte, a netip.Addr) []byte {
	b = append(b, Semantic|NetAddr)

	if !a.IsValid() {
		return append(b, Special|Nil)
	}

	b = e.AppendTag(b, String, 0)
	st := len(b)
	b = a.AppendTo(b)
	b = e.InsertLen(b, st, len(b)-st)

	return b
}

func (e *Encoder) AppendAddrPort(b []byte, a netip.AddrPort) []byte {
	b = append(b, Semantic|NetAddr)

	if !a.IsValid() {
		return append(b, Special|Nil)
	}

	b = e.AppendTag(b, String, 0)
	st := len(b)
	b = a.AppendTo(b)
	b = e.InsertLen(b, st, len(b)-st)

	return b
}

func (e *Encoder) AppendFormat(b []byte, fmt string, args ...interface{}) []byte {
	b = append(b, String)
	st := len(b)

	if fmt == "" {
		b = hfmt.Append(b, args...)
	} else {
		b = hfmt.Appendf(b, fmt, args...)
	}

	l := len(b) - st

	return e.InsertLen(b, st, l)
}

// InsertLen inserts length l before value starting at st copying l bytes forward if needed.
// It is created for AppendFormat because we don't know the final message length.
// But it can be also used in other similar situations.
func (e *Encoder) InsertLen(b []byte, st, l int) []byte {
	if l < 0 {
		panic(l)
	}

	b[st-1] &= TagMask

	if l < Len1 {
		b[st-1] |= byte(l)

		return b
	}

	sz := e.TagSize(l) - 1

	b = append(b, "        "[:sz]...)
	copy(b[st+sz:], b[st:])

	_ = e.AppendTag(b[:st-1], b[st-1], l)

	return b
}

func (e LowEncoder) AppendMap(b []byte, l int) []byte {
	return e.AppendTag(b, Map, l)
}

func (e LowEncoder) AppendArray(b []byte, l int) []byte {
	return e.AppendTag(b, Array, l)
}

func (e LowEncoder) AppendString(b []byte, s string) []byte {
	b = e.AppendTag(b, String, len(s))
	return append(b, s...)
}

func (e LowEncoder) AppendBytes(b, s []byte) []byte {
	b = e.AppendTag(b, Bytes, len(s))
	return append(b, s...)
}

func (e LowEncoder) AppendTagString(b []byte, tag byte, s string) []byte {
	b = e.AppendTag(b, tag, len(s))
	return append(b, s...)
}

func (e LowEncoder) AppendTagBytes(b []byte, tag byte, s []byte) []byte {
	b = e.AppendTag(b, tag, len(s))
	return append(b, s...)
}

func (e LowEncoder) AppendInt(b []byte, v int) []byte {
	if v < 0 {
		return e.AppendTag64(b, Neg, uint64(-v)+1)
	}

	return e.AppendTag64(b, Int, uint64(v))
}

func (e LowEncoder) AppendUint(b []byte, v uint) []byte {
	return e.AppendTag64(b, Int, uint64(v))
}

func (e LowEncoder) AppendInt64(b []byte, v int64) []byte {
	if v < 0 {
		return e.AppendTag64(b, Neg, uint64(-v)+1)
	}

	return e.AppendTag64(b, Int, uint64(v))
}

func (e LowEncoder) AppendUint64(b []byte, v uint64) []byte {
	return e.AppendTag64(b, Int, v)
}

func (e LowEncoder) AppendFloat(b []byte, v float64) []byte {
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

func (e LowEncoder) AppendTag(b []byte, tag byte, v int) []byte {
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

func (e LowEncoder) AppendTag64(b []byte, tag byte, v uint64) []byte {
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

func (e LowEncoder) AppendTagBreak(b []byte, tag byte) []byte {
	return append(b, tag|LenBreak)
}

func (e LowEncoder) AppendSemantic(b []byte, x int) []byte {
	return e.AppendTag(b, Semantic, x)
}

func (e LowEncoder) AppendSpecial(b []byte, x byte) []byte {
	return append(b, Special|x)
}

func (e LowEncoder) AppendBool(b []byte, v bool) []byte {
	if v {
		return append(b, Special|True)
	}

	return append(b, Special|False)
}

func (e LowEncoder) AppendNil(b []byte) []byte {
	return append(b, Special|Nil)
}

func (e LowEncoder) AppendUndefined(b []byte) []byte {
	return append(b, Special|Undefined)
}

func (e LowEncoder) AppendBreak(b []byte) []byte {
	return append(b, Special|Break)
}

func (e LowEncoder) TagSize(v int) int {
	switch {
	case v == -1:
		return 1
	case v < Len1:
		return 1
	case v <= 0xff:
		return 1 + 1
	case v <= 0xffff:
		return 1 + 2
	case v <= 0xffff_ffff:
		return 1 + 4
	default:
		return 1 + 8
	}
}

func (e LowEncoder) Tag64Size(v uint64) int {
	switch {
	case v < Len1:
		return 1
	case v <= 0xff:
		return 1 + 1
	case v <= 0xffff:
		return 1 + 2
	case v <= 0xffff_ffff:
		return 1 + 4
	default:
		return 1 + 8
	}
}
