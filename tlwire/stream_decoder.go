package tlwire

import (
	"io"

	"github.com/nikandfor/errors"
)

type StreamDecoder struct {
	io.Reader

	b    []byte
	i    int
	boff int64
}

const (
	eUnexpectedEOF = -1 - iota
	eBadFormat
	eBadSpecial
)

func NewStreamDecoder(r io.Reader) *StreamDecoder {
	return &StreamDecoder{
		Reader: r,
	}
}

func (d *StreamDecoder) Decode() (data []byte, err error) {
	end, err := d.skipRead()
	if err != nil {
		return nil, err
	}

	st := d.i
	d.i = end

	return d.b[st:end:end], nil
}

func (d *StreamDecoder) Read(p []byte) (n int, err error) {
	end, err := d.skipRead()
	if err != nil {
		return 0, err
	}

	if len(p) < end-d.i {
		return 0, io.ErrShortBuffer
	}

	copy(p, d.b[d.i:end])
	d.i = end

	return len(p), nil
}

func (d *StreamDecoder) WriteTo(w io.Writer) (n int64, err error) {
	for {
		data, err := d.Decode()
		if errors.Is(err, io.EOF) {
			return n, nil
		}
		if err != nil {
			return n, errors.Wrap(err, "decode")
		}

		m, err := w.Write(data)
		n += int64(m)
		if err != nil {
			return n, errors.Wrap(err, "write")
		}
	}
}

func (d *StreamDecoder) skipRead() (end int, err error) {
	for {
		end = d.skip(d.i)
		//	println("skip", d.i, end)
		if end > 0 {
			return end, nil
		}

		if end < eUnexpectedEOF {
			return 0, errors.New("bad format")
		}

		err = d.more()
		if err != nil {
			return 0, err
		}
	}
}

func (d *StreamDecoder) skip(st int) (i int) {
	tag, sub, i := readTag(d.b, st)
	//	println("tag", st, tag, sub, i)
	if i < 0 {
		return i
	}

	switch tag {
	case Int, Neg:
		// already read
	case Bytes, String:
		i += int(sub)
	case Array, Map:
		for el := 0; sub == -1 || el < int(sub); el++ {
			if i == len(d.b) {
				return eUnexpectedEOF
			}
			if sub == -1 && d.b[i] == Special|Break {
				i++
				break
			}

			if tag == Map {
				i = d.skip(i)
				if i < 0 {
					return i
				}
			}

			i = d.skip(i)
			if i < 0 {
				return i
			}
		}
	case Semantic:
		return d.skip(i)
	case Special:
		switch sub {
		case False,
			True,
			Nil,
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
			return eBadSpecial
		}
	}

	if i > len(d.b) {
		return eUnexpectedEOF
	}

	return i
}

func (d *StreamDecoder) more() (err error) {
	{
		copy(d.b, d.b[d.i:])
		d.b = d.b[:len(d.b)-d.i]
		d.boff += int64(d.i)
		d.i = 0
	}

	end := len(d.b)

	if len(d.b) == 0 {
		d.b = make([]byte, 1024)
	} else {
		d.b = append(d.b, 0, 0, 0, 0, 0, 0, 0, 0)
	}

	d.b = d.b[:cap(d.b)]

	n, err := d.Reader.Read(d.b[end:])
	//	println("more", d.i, end, end+n, n, len(d.b))
	d.b = d.b[:end+n]

	if n != 0 && errors.Is(err, io.EOF) {
		err = nil
	}

	return err
}

func readTag(b []byte, st int) (tag byte, sub int64, i int) {
	if st >= len(b) {
		return tag, sub, eUnexpectedEOF
	}

	i = st

	tag = b[i] & TagMask
	sub = int64(b[i] & TagDetMask)
	i++

	if tag == Special {
		return
	}

	if sub < Len1 {
		return
	}

	switch sub {
	case LenBreak:
		sub = -1
	case Len1:
		if i+1 > len(b) {
			return tag, sub, eUnexpectedEOF
		}

		sub = int64(b[i])
		i++
	case Len2:
		if i+2 > len(b) {
			return tag, sub, eUnexpectedEOF
		}

		sub = int64(b[i])<<8 | int64(b[i+1])
		i += 2
	case Len4:
		if i+4 > len(b) {
			return tag, sub, eUnexpectedEOF
		}

		sub = int64(b[i])<<24 | int64(b[i+1])<<16 | int64(b[i+2])<<8 | int64(b[i+3])
		i += 4
	case Len8:
		if i+8 > len(b) {
			return tag, sub, eUnexpectedEOF
		}

		sub = int64(b[i])<<56 | int64(b[i+1])<<48 | int64(b[i+2])<<40 | int64(b[i+3])<<32 |
			int64(b[i+4])<<24 | int64(b[i+5])<<16 | int64(b[i+6])<<8 | int64(b[i+7])
		i += 8
	default:
		return tag, sub, eBadFormat
	}

	return tag, sub, i
}
