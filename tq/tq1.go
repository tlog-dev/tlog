//go:build ignore

package tq

import (
	"fmt"
	"io"

	"github.com/nikandfor/errors"
	"github.com/nikandfor/tlog/low"
	"github.com/nikandfor/tlog/wire"
)

type (
	Filter interface {
		io.Writer

		fmt.Stringer
		SetWriter(io.Writer)
		Result() []byte
	}

	Base struct {
		io.Writer

		wire.Decoder
		wire.Encoder

		B low.Buf
	}

	Literal struct {
		Base
	}

	Length struct {
		Base
	}

	Cat struct {
		Base
	}

	Plus struct {
		Base

		Left  Filter
		Right Filter

		inb low.Buf
	}

	Keys struct {
		Base
	}

	Array struct {
		Base

		Inner Filter
	}
)

func (b *Base) Write(p []byte) (n int, err error) {
	if b.Writer == nil {
		return len(p), nil
	}

	n, err = b.Writer.Write(p)
	if err != nil {
		return n, errors.Wrap(err, "%s", b.Writer)
	}

	return n, nil
}

func (b *Base) SetWriter(w io.Writer) {
	b.Writer = w
}

func (b *Base) Result() []byte { return b.B }

func (f *Literal) Write(p []byte) (_ int, err error) {
	if f.Writer == nil {
		return len(p), nil
	}

	_, err = f.Writer.Write(f.B)
	if err != nil {
		return 0, errors.Wrap(err, "%s", f)
	}

	return len(p), nil
}

func (f *Length) Write(p []byte) (_ int, err error) {
	tag, sub, i := f.Tag(p, 0)

	switch tag {
	case wire.String, wire.Bytes:
		f.B = f.AppendInt64(f.B[:0], sub)
	case wire.Array, wire.Map:
		if sub != -1 {
			f.B = f.AppendInt64(f.B[:0], sub)
			break
		}

		els := 0
		for el := 0; sub == -1 || el < int(sub); el++ {
			if sub == -1 && f.Break(p, &i) {
				break
			}

			if tag == wire.Map {
				i = f.Skip(p, i)
			}
			i = f.Skip(p, i)

			els++
		}

		f.B = f.AppendInt(f.B[:0], els)
	default:
		return 0, errors.New("expected one of string, array or map")
	}

	return f.Base.Write(f.B)
}

func (f *Cat) Write(p []byte) (_ int, err error) {
	tag, sub, i := f.Tag(p, 0)
	if tag != wire.Array {
		return 0, errors.New("array expected")
	}

	f.B = f.AppendTag(f.B[:0], wire.String, 0)

	st := len(f.B)

	var firstTag byte

	var s []byte
	for el := 0; sub == -1 || el < int(sub); el++ {
		if sub == -1 && f.Break(p, &i) {
			break
		}

		tag, _, _ = f.Tag(p, i)
		if tag != wire.String && tag != wire.Bytes {
			return 0, errors.New("array of strings expected")
		}
		if el == 0 {
			firstTag = tag
		}

		s, i = f.Decoder.String(p, i)

		f.B = append(f.B, s...)
	}

	_ = f.AppendTag(f.B[:0], firstTag, 0)
	f.B = f.InsertLen(f.B, st, len(f.B)-st)

	return f.Base.Write(f.B)
}

func (f *Keys) Write(p []byte) (i int, err error) {
	tag, sub, i := f.Tag(p, 0)
	if tag != wire.Map {
		return 0, errors.New("map expected")
	}

	f.B = f.AppendArray(f.B[:0], int(sub))

	for el := 0; sub == -1 || el < int(sub); el++ {
		if sub == -1 && f.Break(p, &i) {
			break
		}

		st := i
		i = f.Skip(p, i)

		f.B = append(f.B, p[st:i]...)

		i = f.Skip(p, i)
	}

	return f.Base.Write(f.B)
}

func (f *Array) Write(p []byte) (i int, err error) {
	tag, sub, i := f.Tag(p, 0)
	if tag != wire.Array {
		return 0, errors.New("array expected")
	}

	f.B = f.AppendArray(f.B[:0], -1)

	f.Inner.SetWriter(&f.B)

	for el := 0; sub == -1 || el < int(sub); el++ {
		if sub == -1 && f.Break(p, &i) {
			break
		}

		st := i

		i = f.Skip(p, i)

		_, err = f.Inner.Write(p[st:i])
		if err != nil {
			return 0, errors.Wrap(err, f.Inner.String())
		}
	}

	return f.Base.Write(f.B)
}

func (f *Plus) Write(p []byte) (_ int, err error) {
	f.inb = f.inb[:0]

	f.Left.SetWriter(&f.inb)

	_, err = f.Left.Write(p)
	if err != nil {
		return 0, errors.Wrap(err, "left: %v", f.Left)
	}

	st := len(f.inb)

	f.Right.SetWriter(&f.inb)

	_, err = f.Right.Write(p)
	if err != nil {
		return 0, errors.Wrap(err, "right: %v", f.Right)
	}

	lt, ls, _ := f.Tag(f.inb, 0)

	rt, rs, ri := f.Tag(f.inb, st)

	switch lt {
	case wire.Int, wire.Neg:
		if rt != wire.Int && rt != wire.Neg {
			return 0, errors.New("can't add %v and %v", wire.Tag(lt), wire.Tag(rt))
		}

		f.inb = f.AppendInt64(f.inb[:0], ls+rs)
	case wire.String, wire.Bytes:
		if rt != wire.String && rt != wire.Bytes {
			return 0, errors.New("can't add %v and %v", wire.Tag(lt), wire.Tag(rt))
		}

		n := copy(f.inb[st:], f.inb[ri:])
		f.inb = f.inb[:st+n]
	default:
		return 0, errors.New("can't add %v and %v", wire.Tag(lt), wire.Tag(rt))
	}

	_, err = f.Writer.Write(f.inb)
	if err != nil {
		return 0, errors.Wrap(err, "%s", f.Writer)
	}

	return len(p), nil
}

func (f *Literal) String() string { return "literal" }

func (f *Length) String() string { return "length" }

func (f *Cat) String() string { return "cat" }

func (f *Keys) String() string { return "keys" }

func (f *Array) String() string { return "array" }
