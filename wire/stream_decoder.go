package wire

import (
	"io"

	"github.com/nikandfor/errors"
)

type StreamDecoder struct {
	io.Reader

	b   []byte
	i   int
	ref int64
}

func NewStreamDecoder(r io.Reader) *StreamDecoder {
	return &StreamDecoder{
		Reader: r,
	}
}

func (d *StreamDecoder) Decode() (data []byte, err error) {
	if d.i != 0 {
		d.move()
	}

	st := d.i

	end, err := d.skip(st)
	if err != nil {
		return d.b[st:], errors.Wrap(err, "st %x end %x", d.ref+int64(st), d.ref+int64(end))
	}

	d.i = end

	return d.b[st:end], nil
}

func (d *StreamDecoder) skip(st int) (i int, err error) {
	tag, sub, i, err := d.tag(st)
	//	defer func() {
	//		fmt.Fprintf(os.Stderr, "skip [%x -> %x]  tag %x %x  err %v\n", st, i, tag, sub, err)
	//	}()
	if err != nil {
		return
	}

	switch tag {
	case Int, Neg:
		// already read
	case String, Bytes:
		i += int(sub)
	case Array, Map:
		for el := 0; sub == -1 || el < int(sub); el++ {
			if sub == -1 {
				if i >= len(d.b) {
					err = d.more(i + 1)
					if err != nil {
						return
					}
				}

				if d.b[i] == Special|Break {
					i++
					break
				}
			}

			if tag == Map {
				i, err = d.skip(i)
				if err != nil {
					return
				}
			}

			i, err = d.skip(i)
			if err != nil {
				return
			}
		}
	case Semantic:
		i, err = d.skip(i)
		if err != nil {
			return
		}
	case Special:
		switch sub {
		case False,
			True,
			Null,
			Undefined,
			Break:
		case Float8:
			i += 1 //nolint:revive
		case Float16:
			i += 2
		case Float32:
			i += 4
		case Float64:
			i += 8
		default:
			err = errors.New("unsupported special")
		}
	}

	if err != nil {
		return
	}

	if i > len(d.b) {
		err = d.more(i)
	}

	return
}

func (d *StreamDecoder) tag(st int) (tag byte, sub int64, i int, err error) {
	i = st

	if i >= len(d.b) {
		err = d.more(i + 1)
		if err != nil {
			return
		}
	}

	tag = d.b[i] & TagMask
	sub = int64(d.b[i] & TagDetMask)
	i++

	if tag == Special {
		return
	}

	switch {
	case sub < Len1:
		// we are ok
	case sub == Len1:
		if i+1 > len(d.b) {
			err = d.more(i + 1)
			if err != nil {
				return
			}
		}

		sub = int64(d.b[i])
		i++
	case sub == Len2:
		if i+2 > len(d.b) {
			err = d.more(i + 2)
			if err != nil {
				return
			}
		}

		sub = int64(d.b[i])<<8 | int64(d.b[i+1])
		i += 2
	case sub == Len4:
		if i+4 > len(d.b) {
			err = d.more(i + 4)
			if err != nil {
				return
			}
		}

		sub = int64(d.b[i])<<24 | int64(d.b[i+1])<<16 | int64(d.b[i+2])<<8 | int64(d.b[i+3])
		i += 4
	case sub == Len8:
		if i+8 > len(d.b) {
			err = d.more(i + 8)
			if err != nil {
				return
			}
		}

		sub = int64(d.b[i])<<56 | int64(d.b[i+1])<<48 | int64(d.b[i+2])<<40 | int64(d.b[i+3])<<32 |
			int64(d.b[i+4])<<24 | int64(d.b[i+5])<<16 | int64(d.b[i+6])<<8 | int64(d.b[i+7])
		i += 8
	case sub == LenBreak:
		sub = -1
	default:
		err = errors.New("malformed message")
	}

	return
}

func (d *StreamDecoder) more(l int) (err error) {
	if d.Reader == nil {
		return errors.New("end of buffer")
	}

	end := len(d.b)

	if l > cap(d.b) {
		d.grow(l)
	}

	d.b = d.b[:cap(d.b)]

	n, err := io.ReadAtLeast(d.Reader, d.b[end:], l-end)

	end += n
	d.b = d.b[:end]

	return err
}

func (d *StreamDecoder) grow(l int) {
	n := 4096

	for n < l {
		if n < 0x10000 {
			n *= 2
		} else {
			n += n / 4
		}
	}

	q := make([]byte, n)
	copy(q, d.b)

	d.b = q
}

func (d *StreamDecoder) move() {
	copy(d.b, d.b[d.i:])

	d.ref += int64(d.i)

	d.b = d.b[:len(d.b)-d.i]
	d.i = 0
}
