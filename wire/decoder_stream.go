package wire

import (
	"io"
	"math"

	"github.com/nikandfor/errors"
)

type (
	StreamDecoder struct {
		io.Reader

		b   []byte
		i   int
		ref int64

		keep bool

		err error
	}
)

func (d *StreamDecoder) Ref() int64 { return d.ref }
func (d *StreamDecoder) I() int     { return d.i }

func (d *StreamDecoder) Pos() int64 { return d.ref + int64(d.i) }

func NewStreamDecoder(r io.Reader) *StreamDecoder {
	return &StreamDecoder{Reader: r}
}

func (d *StreamDecoder) Keep(en bool) int64 {
	if d.err != nil {
		return -1
	}

	if !en {
		d.keep = false
		return d.Pos()
	}

	d.keep = false

	if !d.more(0) {
		return -1
	}

	d.keep = true

	return d.Pos()
}

func (d *StreamDecoder) Bytes() []byte {
	if !d.keep {
		panic("not keeping")
	}

	return d.b[:d.i]
}

func (d *StreamDecoder) Skip() {
	tag, sub := d.PeekTag()

	//	println(fmt.Sprintf("Skip %2x  -> %2x %2x _   data % .10x (%.10q)  from %v  err %v", d.i, tag, sub, d.b[d.i:], d.b[d.i:], loc.Callers(1, 4), d.err))

	switch tag {
	case Int, Neg:
		d.Int()
	case String, Bytes:
		d.String()
	case Array, Map:
		d.i++

		for el := 0; sub == -1 || el < int(sub); el++ {
			if sub == -1 && d.Break() {
				break
			}

			d.String()
			if tag == Map {
				d.Skip()
			}
		}
	case Semantic:
		d.i++

		d.Skip()
	case Special:
		d.i++

		switch sub {
		case False,
			True,
			Null,
			Undefined,
			Break:
		case Float8:
			d.i += 1
		case Float16:
			d.i += 2
		case Float32:
			d.i += 4
		case Float64:
			d.i += 8
		default:
			d.newErr("bad special")
		}
	}

	return
}

func (d *StreamDecoder) Break() bool {
	if d.err != nil {
		return false
	}

	if d.i >= len(d.b) && !d.more(1) {
		return false
	}

	if d.b[d.i] != Special|Break {
		return false
	}

	d.i++

	return true
}

func (d *StreamDecoder) String() (v []byte, tag byte) {
	tag, l := d.Tag()
	if d.err != nil {
		return
	}

	//	println(fmt.Sprintf("string %x %x  -> %x + %x / %x", tag, l, d.ref, d.i, len(d.b)))

	if d.i+int(l) >= len(d.b) && !d.more(int(l)) {
		return
	}

	v = d.b[d.i : d.i+int(l)]

	d.i += int(l)

	return
}

func (d *StreamDecoder) PeekTag() (tag byte, sub int64) {
	tag, sub, _ = d.peekTag()
	return
}

func (d *StreamDecoder) Tag() (tag byte, sub int64) {
	tag, sub, i := d.peekTag()
	d.i = i
	return
}

func (d *StreamDecoder) peekTag() (tag byte, sub int64, i int) {
	if d.err != nil {
		return
	}

	if d.i >= len(d.b) && !d.more(1) {
		return
	}

	i = d.i

	tag = d.b[i]
	sub = int64(tag & TagDetMask)
	tag &= TagMask
	i++

	//	println(fmt.Sprintf("tag  %2x  -> %2x %2x %2x  data % .10x  from %v", d.i, tag, sub, i, d.b[d.i:], loc.Callers(1, 3)))

	if tag == Special {
		return
	}

	switch {
	case sub < Len1:
		// we are ok
	case sub == LenBreak:
		sub = -1
	case sub == Len1:
		if i+1 > len(d.b) && !d.more(2) {
			return
		}

		sub = int64(d.b[i])
		i++
	case sub == Len2:
		if i+2 > len(d.b) && !d.more(3) {
			return
		}

		sub = int64(d.b[i])<<8 | int64(d.b[i+1])
		i += 2
	case sub == Len4:
		if i+4 > len(d.b) && !d.more(5) {
			return
		}

		sub = int64(d.b[i])<<24 | int64(d.b[i+1])<<16 | int64(d.b[i+2])<<8 | int64(d.b[i+3])
		i += 4
	case sub == Len8:
		if i+8 > len(d.b) && !d.more(9) {
			return
		}

		sub = int64(d.b[i])<<56 | int64(d.b[i+1])<<48 | int64(d.b[i+2])<<40 | int64(d.b[i+3])<<32 |
			int64(d.b[i+4])<<24 | int64(d.b[i+5])<<16 | int64(d.b[i+6])<<8 | int64(d.b[i+7])
		i += 8
	default:
		d.newErr("bad int")
	}

	return
}

func (d *StreamDecoder) Signed() (v int64) {
	tag, v := d.Tag()

	if tag == Neg {
		v = -v
	}

	return
}

func (d *StreamDecoder) Int() uint64 {
	_, v := d.Tag()

	return uint64(v)
}

func (d *StreamDecoder) Float() (v float64) {
	if d.err != nil {
		return
	}

	if d.i >= len(d.b) && !d.more(1) {
		return
	}

	sub := int(d.b[d.i]) & TagDetMask
	d.i++

	switch {
	case sub == Float8:
		v = float64(d.b[d.i])
		d.i++
	case sub == Float32:
		v = float64(math.Float32frombits(
			uint32(d.b[d.i])<<24 | uint32(d.b[d.i+1])<<16 | uint32(d.b[d.i+2])<<8 | uint32(d.b[d.i+3]),
		))

		d.i += 4
	case sub == Float64:
		v = math.Float64frombits(
			uint64(d.b[d.i])<<56 | uint64(d.b[d.i+1])<<48 | uint64(d.b[d.i+2])<<40 | uint64(d.b[d.i+3])<<32 |
				uint64(d.b[d.i+4])<<24 | uint64(d.b[d.i+5])<<16 | uint64(d.b[d.i+6])<<8 | uint64(d.b[d.i+7]),
		)

		d.i += 8
	default:
		d.newErr("bad float")
	}

	return
}

func (d *StreamDecoder) Err() error {
	return d.err
}

func (d *StreamDecoder) ResetErr() {
	d.err = nil
}

func (d *StreamDecoder) wrapErr(err error, f string, args ...interface{}) {
	if d.err != nil {
		return
	}

	d.err = errors.WrapDepth(err, 1, f, args...)
	d.err = errors.WrapNoLoc(d.err, "(pos %x)", d.ref+int64(d.i))
}

func (d *StreamDecoder) newErr(f string, args ...interface{}) {
	if d.err != nil {
		return
	}

	d.err = errors.NewDepth(1, f, args...)
	d.err = errors.WrapNoLoc(d.err, "(pos %x)", d.ref+int64(d.i))
}

func (d *StreamDecoder) more(l int) bool {
	if d.Reader == nil {
		d.wrapErr(io.ErrUnexpectedEOF, "short buffer, no reader")
		return false
	}

	end := len(d.b)

	if false && d.i > 0 && !d.keep {
		copy(d.b, d.b[d.i:])

		d.ref += int64(d.i)
		end -= d.i
		d.i = 0

		d.b = d.b[:end]
	}

	for end+l > cap(d.b) {
		d.b = append(d.b[:cap(d.b)], 0, 0, 0, 0)
	}

	d.b = d.b[:cap(d.b)]

	n, err := io.ReadAtLeast(d.Reader, d.b[end:], d.i+l-end)
	//	println(fmt.Sprintf("more  ref %x i %x lim %x  end %x -> n %x  space %x cap %x err %v", d.ref, d.i, l, end, n, len(d.b[end:]), cap(d.b), err))
	d.b = d.b[:end+n]

	if err != nil {
		d.wrapErr(err, "read")
	}

	return err == nil
}
