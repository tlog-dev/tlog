package tlwire

import (
	"math"
	"net/netip"
	"time"
)

type (
	Decoder struct {
		LowDecoder
	}

	LowDecoder struct{}
)

func (d *Decoder) Time(p []byte, st int) (t time.Time, i int) {
	if p[st] != Semantic|Time {
		panic("not a time")
	}

	tag, sub, i := d.Tag(p, st+1)

	if tag == Int {
		return time.Unix(0, sub), i
	}

	if tag != Map || sub == -1 {
		panic("unsupported time")
	}

	var (
		k     []byte
		ts    int64
		tzN   []byte
		tzOff int64
	)

	for el := 0; el < int(sub); el++ {
		k, i = d.Bytes(p, i)

		switch string(k) {
		case "t":
			ts, i = d.Signed(p, i)
		case "z":
			if p[i] != Array|2 {
				panic("unsupported time zone")
			}
			i++

			tzN, i = d.Bytes(p, i)
			tzOff, i = d.Signed(p, i)
		default:
			i = d.Skip(p, i)
		}
	}

	if ts != 0 {
		t = time.Unix(0, ts)
	}

	if tzN != nil || tzOff != 0 {
		l := time.FixedZone(string(tzN), int(tzOff))
		t = t.In(l)
	}

	return
}

func (d *Decoder) Timestamp(p []byte, st int) (ts int64, i int) {
	if p[st] != Semantic|Time {
		panic("not a time")
	}

	tag, sub, i := d.Tag(p, st+1)

	if tag == Int {
		return sub, i
	}

	if tag != Map || sub == -1 {
		panic("unsupported time")
	}

	var k []byte

	for el := 0; el < int(sub); el++ {
		k, i = d.Bytes(p, i)

		switch string(k) {
		case "t":
			ts, i = d.Signed(p, i)
		default:
			i = d.Skip(p, i)
		}
	}

	return
}

func (d *Decoder) Duration(p []byte, st int) (dr time.Duration, i int) {
	if p[st] != Semantic|Duration {
		panic("not a duration")
	}

	tag, sub, i := d.Tag(p, st+1)

	if tag != Int && tag != Neg {
		panic("unsupported duration")
	}

	if tag == Neg {
		sub = -sub
	}

	return time.Duration(sub), i
}

func (d *Decoder) Addr(p []byte, st int) (a netip.Addr, ap netip.AddrPort, i int, err error) {
	if p[st] != Semantic|NetAddr {
		panic("not an address")
	}

	tag, sub, i := d.Tag(p, st+1)
	if tag == Special && sub == Nil {
		return
	}
	if tag != String {
		panic("unsupported address encoding")
	}

	ab := p[i : i+int(sub)]
	i += int(sub)

	err = a.UnmarshalText(ab)
	if err != nil {
		err = ap.UnmarshalText(ab)
	}
	if err != nil {
		return a, ap, st, err
	}

	return
}

func (d LowDecoder) Skip(b []byte, st int) (i int) {
	_, _, i = d.SkipTag(b, st)
	return
}

func (d LowDecoder) SkipTag(b []byte, st int) (tag byte, sub int64, i int) {
	tag, sub, i = d.Tag(b, st)

	//	println(fmt.Sprintf("Skip %x  tag %x %x %x  data % x", st, tag, sub, i, b[st:]))

	switch tag {
	case Int, Neg:
		_, i = d.Unsigned(b, st)
	case String, Bytes:
		_, i = d.Bytes(b, st)
	case Array, Map:
		for el := 0; sub == -1 || el < int(sub); el++ {
			if sub == -1 && d.Break(b, &i) {
				break
			}

			if tag == Map {
				i = d.Skip(b, i)
			}

			i = d.Skip(b, i)
		}
	case Semantic:
		i = d.Skip(b, i)
	case Special:
		switch sub {
		case False,
			True,
			Nil,
			Undefined,
			None,
			Hidden,
			SelfRef,
			Break:
		case Float8, Float16, Float32, Float64:
			i += 1 << (int(sub) - Float8)
		default:
			panic("unsupported special")
		}
	}

	return
}

func (d LowDecoder) Break(b []byte, i *int) bool {
	if b[*i] != Special|Break {
		return false
	}

	*i++

	return true
}

func (d LowDecoder) Bytes(b []byte, st int) (v []byte, i int) {
	_, l, i := d.Tag(b, st)

	return b[i : i+int(l)], i + int(l)
}

func (d LowDecoder) TagOnly(b []byte, st int) (tag byte) {
	return b[st] & TagMask
}

func (d LowDecoder) Tag(b []byte, st int) (tag byte, sub int64, i int) {
	i = st

	tag = b[i] & TagMask
	sub = int64(b[i] & TagDetMask)
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

func (d LowDecoder) Signed(b []byte, st int) (v int64, i int) {
	u, i := d.Unsigned(b, st)

	if b[st]&TagMask == Int {
		return int64(u), i
	}

	return 1 - int64(u), i
}

func (d LowDecoder) Unsigned(b []byte, st int) (v uint64, i int) {
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

func (d LowDecoder) Float(b []byte, st int) (v float64, i int) {
	i = st

	st = int(b[i]) & TagDetMask
	i++

	switch {
	case st == Float8:
		v = float64(int8(b[i]))
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
