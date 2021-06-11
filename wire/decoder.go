package wire

import (
	"fmt"
	"math"
	"time"

	"github.com/nikandfor/loc"
)

type (
	Decoder struct {
		LowDecoder
	}

	LowDecoder struct {
	}
)

func (d *Decoder) Time(p []byte, st int) (t time.Time, i int) {
	if p[st] != Semantic|Time {
		panic("not a time")
	}

	tag, sub, i := d.Tag(p, st+1)

	//	println(fmt.Sprintf("Time %x  tag %x %x %x  data % x", st, tag, sub, i, p[st:]))

	switch tag {
	case Int:
		t = time.Unix(0, sub)
	default:
		panic("unsupported time")
	}

	return
}

func (d *Decoder) Caller(p []byte, st int) (pc loc.PC, i int) {
	if p[st] != Semantic|Caller {
		panic("not a caller")
	}
	i = st + 1

first:
	tag, sub, i := d.Tag(p, i)

	switch tag {
	case Int:
		pc = loc.PC(sub)

		if pc != 0 && !loc.Cached(pc) {
			loc.SetCache(pc, "_", ".", 0)
		}

		return
	case Array:
		if sub == 0 {
			return
		}

		goto first
	case Map:
	default:
		panic(fmt.Sprintf("unsupported caller tag: %x", tag))
	}

	var v uint64
	var k []byte
	var name, file string
	var line int

	for el := 0; el < int(sub); el++ {
		k, i = d.String(p, i)

		switch string(k) {
		case "p":
			v, i = d.Int(p, i)

			pc = loc.PC(v)
		case "l":
			v, i = d.Int(p, i)

			line = int(v)
		case "n":
			k, i = d.String(p, i)

			name = string(k)
		case "f":
			k, i = d.String(p, i)

			file = string(k)
		}
	}

	if pc == 0 {
		return
	}

	loc.SetCache(pc, name, file, line)

	return
}

func (d *LowDecoder) Skip(b []byte, st int) (i int) {
	tag, sub, i := d.Tag(b, st)

	//	println(fmt.Sprintf("Skip %x  tag %x %x %x  data % x", st, tag, sub, i, b[st:]))

	switch tag {
	case Int, Neg:
		_, i = d.Int(b, st)
	case String, Bytes:
		_, i = d.String(b, st)
	case Array, Map:
		for el := 0; sub == -1 || el < int(sub); el++ {
			if sub == -1 && d.Break(b, &i) {
				break
			}

			if tag == Map {
				_, i = d.String(b, i)
			}

			i = d.Skip(b, i)
		}
	case Semantic:
		i = d.Skip(b, i)
	case Special:
		switch sub {
		case False,
			True,
			Null,
			Undefined,
			Break:
		case Float8:
			i += 1
		case Float16:
			i += 2
		case Float32:
			i += 4
		case Float64:
			i += 8
		default:
			panic("unsupported special")
		}
	}

	return
}

func (_ *LowDecoder) Break(b []byte, i *int) bool {
	if b[*i] != Special|Break {
		return false
	}

	*i++

	return true
}

func (d *LowDecoder) String(b []byte, st int) (v []byte, i int) {
	_, l, i := d.Tag(b, st)

	return b[i : i+int(l)], i + int(l)
}

func (_ *LowDecoder) TagOnly(b []byte, st int) (tag byte) {
	return b[st] & TagMask
}

func (_ *LowDecoder) Tag(b []byte, st int) (tag byte, sub int64, i int) {
	i = st

	tag = b[i]
	sub = int64(tag & TagDetMask)
	tag &= TagMask
	i++

	if tag == Special {
		return
	}

	switch {
	case sub < Len1:
		// we are ok
	case sub == LenBreak:
		sub = -1
	case sub == Len1:
		sub = int64(b[i])
		i++
	case sub == Len2:
		sub = int64(b[i])<<8 | int64(b[i+1])
		i += 2
	case sub == Len4:
		sub = int64(b[i])<<24 | int64(b[i+1])<<16 | int64(b[i+2])<<8 | int64(b[i+3])
		i += 4
	case sub == Len8:
		sub = int64(b[i])<<56 | int64(b[i+1])<<48 | int64(b[i+2])<<40 | int64(b[i+3])<<32 |
			int64(b[i+4])<<24 | int64(b[i+5])<<16 | int64(b[i+6])<<8 | int64(b[i+7])
		i += 8
	default:
		panic("malformed message")
	}

	return
}

func (_ *LowDecoder) Signed(b []byte, st int) (v int64, i int) {
	i = st

	st = int(b[i]) & TagMask
	v = int64(b[i]) & TagDetMask
	i++

	switch {
	case v < Len1:
		// we are ok
	case v == Len1:
		v = int64(b[i])
		i++
	case v == Len2:
		v = int64(b[i])<<8 | int64(b[i+1])
		i += 2
	case v == Len4:
		v = int64(b[i])<<24 | int64(b[i+1])<<16 | int64(b[i+2])<<8 | int64(b[i+3])
		i += 4
	case v == Len8:
		v = int64(b[i])<<56 | int64(b[i+1])<<48 | int64(b[i+2])<<40 | int64(b[i+3])<<32 |
			int64(b[i+4])<<24 | int64(b[i+5])<<16 | int64(b[i+6])<<8 | int64(b[i+7])
		i += 8
	default:
		panic("malformed message")
	}

	if st == Neg {
		v = -v
	}

	return
}

func (_ *LowDecoder) Int(b []byte, st int) (v uint64, i int) {
	i = st

	v = uint64(b[i]) & TagDetMask
	i++

	switch {
	case v < Len1:
		// we are ok
	case v == Len1:
		v = uint64(b[i])
		i++
	case v == Len2:
		v = uint64(b[i])<<8 | uint64(b[i+1])
		i += 2
	case v == Len4:
		v = uint64(b[i])<<24 | uint64(b[i+1])<<16 | uint64(b[i+2])<<8 | uint64(b[i+3])
		i += 4
	case v == Len8:
		v = uint64(b[i])<<56 | uint64(b[i+1])<<48 | uint64(b[i+2])<<40 | uint64(b[i+3])<<32 |
			uint64(b[i+4])<<24 | uint64(b[i+5])<<16 | uint64(b[i+6])<<8 | uint64(b[i+7])
		i += 8
	default:
		panic("malformed message")
	}

	return
}

func (_ *LowDecoder) Float(b []byte, st int) (v float64, i int) {
	i = st

	st = int(b[i]) & TagDetMask
	i++

	switch {
	case st == Float8:
		v = float64(b[i])
		i++
	case st == Float32:
		v = float64(math.Float32frombits(
			uint32(b[i])<<24 | uint32(b[i+1])<<16 | uint32(b[i+2])<<8 | uint32(b[i+3]),
		))

		i += 4
	case st == Float64:
		v = math.Float64frombits(
			uint64(b[i])<<56 | uint64(b[i+1])<<48 | uint64(b[i+2])<<40 | uint64(b[i+3])<<32 |
				uint64(b[i+4])<<24 | uint64(b[i+5])<<16 | uint64(b[i+6])<<8 | uint64(b[i+7]),
		)

		i += 8
	default:
		panic("malformed message")
	}

	return
}
