package tlz

import (
	"bytes"
	"io"

	"github.com/nikandfor/errors"
	"github.com/nikandfor/hacked/hfmt"

	"github.com/nikandfor/tlog/low"
)

type (
	Decoder struct {
		io.Reader

		// output
		block []byte
		mask  int
		pos   int64 // output stream pos

		// current tag
		state    byte
		off, len int

		// input
		b    []byte
		i    int
		boff int64 // input stream offset to b[0]
	}

	Dumper struct {
		io.Writer

		d Decoder

		GlobalOffset int64

		b low.Buf
	}
)

var eUnexpectedEOF = errors.NewNoCaller("need more")

func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{
		Reader: r,
	}
}

func NewDecoderBytes(b []byte) *Decoder {
	return &Decoder{
		b: b,
	}
}

func (d *Decoder) Reset(rd io.Reader) {
	d.ResetBytes(nil)
	d.Reader = rd
}

func (d *Decoder) ResetBytes(b []byte) {
	d.Reader = nil

	if b != nil {
		d.b = b
	}

	d.i = 0
	d.b = d.b[:len(b)]
	d.boff = 0

	d.state = 0
}

func (d *Decoder) Read(p []byte) (n int, err error) {
	var m, i int

	for n < len(p) && err == nil {
		m, i, err = d.read(p[n:], d.i)

		n += m
		d.i = i

		if n == len(p) {
			err = nil
			break
		}

		if err != eUnexpectedEOF { //nolint:errorlint
			continue
		}

		err = d.more()
		if errors.Is(err, io.EOF) && (d.state != 0 || d.i < len(d.b)) {
			err = io.ErrUnexpectedEOF
		}
	}

	return n, err
}

func (d *Decoder) read(p []byte, st int) (n, i int, err error) {
	//	defer func() { println("eazy.Decoder.read", st, i, n, err, len(d.b)) }()
	if d.state != 0 && len(d.block) == 0 {
		return 0, st, errors.New("missed meta")
	}

	i = st

	for d.state == 0 {
		i, err = d.readTag(i)
		if err != nil {
			return
		}
	}

	if d.state == 'l' && i == len(d.b) {
		return 0, i, eUnexpectedEOF
	}

	end := d.len
	if end > len(p) {
		end = len(p)
	}

	if d.state == 'l' {
		end = copy(p[:end], d.b[i:])
		i += end
	} else {
		end = copy(p[:end], d.block[d.off&d.mask:])
		d.off += end
	}

	d.len -= end

	for n < end {
		m := copy(d.block[int(d.pos)&d.mask:], p[n:end])
		n += m
		d.pos += int64(m)
	}

	if d.len == 0 {
		d.state = 0
	}

	return
}

func (d *Decoder) readTag(st int) (i int, err error) {
	tag, l, i, err := d.tag(d.b, st)
	if err != nil {
		return st, err
	}

	//	println("readTag", tag, l, st, i, d.i, len(d.b))

	if tag == Literal && l == Meta {
		return d.continueMetaTag(i)
	}

	switch tag {
	case Literal:
		d.state = 'l'
		d.len = l
	case Copy:
		d.off, i, err = d.roff(d.b, i)
		if err != nil {
			return st, err
		}

		d.off = int(d.pos) - d.off - l

		d.state = 'c'
		d.len = l
	default:
		return st, errors.New("unsupported tag: %x", tag)
	}

	return i, nil
}

func (d *Decoder) continueMetaTag(st int) (i int, err error) {
	i = st
	st--

	if i == len(d.b) {
		return st, eUnexpectedEOF
	}

	{ // legacy fallback
		const legacy = "\x00\x03tlz\x00\x13000\x00\x20"

		if len(d.b) < len(legacy)+1 && bytes.Equal(d.b, []byte(legacy)[:len(d.b)]) {
			return st, eUnexpectedEOF
		}

		if bytes.Equal(d.b[:len(legacy)], []byte(legacy)) {
			i = len(legacy)

			bs := int(d.b[i])
			i++

			bs = 1 << bs

			if cap(d.block) >= bs {
				d.block = d.block[:bs]

				for i := 0; i < bs; {
					i += copy(d.block[i:], zeros)
				}
			} else {
				d.block = make([]byte, bs)
			}

			d.pos = 0
			d.mask = bs - 1

			d.state = 0

			return i, nil
		}
	}

	meta := d.b[i]
	i++

	l := int(meta &^ MetaTagMask)

	if l == 7 {
		if i == len(d.b) {
			return st, eUnexpectedEOF
		}

		l = int(d.b[i])
		i++
	} else {
		l = 1 << l
	}

	//	println("meta", st-1, i, meta, l, i+l, len(d.b))

	if i+l > len(d.b) {
		return st, eUnexpectedEOF
	}

	switch meta & MetaTagMask {
	case MetaMagic:
		if !bytes.Equal(d.b[i:i+l], []byte("eazy")) {
			return st, errors.New("bad magic")
		}
	case MetaReset:
		bs := int(d.b[i])
		bs = 1 << bs

		if cap(d.block) >= bs {
			d.block = d.block[:bs]

			for i := 0; i < bs; {
				i += copy(d.block[i:], zeros)
			}
		} else {
			d.block = make([]byte, bs)
		}

		d.pos = 0
		d.mask = bs - 1

		d.state = 0
	default:
		return st, errors.New("unsupported meta: %x", meta)
	}

	i += l

	return i, nil
}

func (d *Decoder) roff(b []byte, st int) (off, i int, err error) {
	if st >= len(b) {
		return 0, st, eUnexpectedEOF
	}

	i = st

	off = int(b[i])
	i++

	switch off {
	case Off1:
		if i+1 > len(b) {
			return off, st, eUnexpectedEOF
		}

		off = int(b[i])
		i++
	case Off2:
		if i+2 > len(b) {
			return off, st, eUnexpectedEOF
		}

		off = int(b[i])<<8 | int(b[i+1])
		i += 2
	case Off4:
		if i+4 > len(b) {
			return off, st, eUnexpectedEOF
		}

		off = int(b[i])<<24 | int(b[i+1])<<16 | int(b[i+2])<<8 | int(b[i+3])
		i += 4
	case Off8:
		if i+8 > len(b) {
			return off, st, eUnexpectedEOF
		}

		off = int(b[i])<<56 | int(b[i+1])<<48 | int(b[i+2])<<40 | int(b[i+3])<<32 |
			int(b[i+4])<<24 | int(b[i+5])<<16 | int(b[i+6])<<8 | int(b[i+7])
		i += 8
	default:
		// off is embedded
	}

	return off, i, nil
}

func (d *Decoder) tag(b []byte, st int) (tag, l, i int, err error) {
	if st >= len(b) {
		return 0, 0, st, eUnexpectedEOF
	}

	i = st

	tag = int(b[i]) & TagMask
	l = int(b[i]) & TagLenMask
	i++

	switch l {
	case Len1:
		if i+1 > len(b) {
			return tag, l, st, eUnexpectedEOF
		}

		l = int(b[i])
		i++
	case Len2:
		if i+2 > len(b) {
			return tag, l, st, eUnexpectedEOF
		}

		l = int(b[i])<<8 | int(b[i+1])
		i += 2
	case Len4:
		if i+4 > len(b) {
			return tag, l, st, eUnexpectedEOF
		}

		l = int(b[i])<<24 | int(b[i+1])<<16 | int(b[i+2])<<8 | int(b[i+3])
		i += 4
	case Len8:
		if i+8 > len(b) {
			return tag, l, st, eUnexpectedEOF
		}

		l = int(b[i])<<56 | int(b[i+1])<<48 | int(b[i+2])<<40 | int(b[i+3])<<32 |
			int(b[i+4])<<24 | int(b[i+5])<<16 | int(b[i+6])<<8 | int(b[i+7])
		i += 8
	default:
		// l is embedded
	}

	return tag, l, i, nil
}

func (d *Decoder) more() (err error) {
	if d.Reader == nil {
		return io.EOF
	}

	copy(d.b, d.b[d.i:])
	d.b = d.b[:len(d.b)-d.i]
	d.boff += int64(d.i)
	d.i = 0

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

func Dump(p []byte) string {
	var d Dumper

	_, err := d.Write(p)
	if err != nil {
		return err.Error()
	}

	return string(d.b)
}

func NewDumper(w io.Writer) *Dumper {
	return &Dumper{
		Writer: w,
	}
}

func (w *Dumper) Write(p []byte) (i int, err error) {
	w.b = w.b[:0]

	var tag, l int

	for i < len(p) {
		if w.GlobalOffset >= 0 {
			w.b = hfmt.Appendf(w.b, "%6x  ", int(w.GlobalOffset)+i)
		}

		w.b = hfmt.Appendf(w.b, "%4x  ", i)

		w.b = hfmt.Appendf(w.b, "%6x  ", w.d.pos)

		st := i

		tag, l, i, err = w.d.tag(p, i)
		if err != nil {
			return st, err
		}

		//	println("loop", i, tag>>7, l)

		switch {
		case l == Meta:
			if i == len(p) {
				return st, eUnexpectedEOF
			}

			tag = int(p[i])
			i++

			l = tag &^ MetaTagMask

			if l == 7 {
				if i == len(p) {
					return st, eUnexpectedEOF
				}

				l = int(p[i])
				i++
			} else {
				l = 1 << l
			}

			w.b = hfmt.Appendf(w.b, "meta %2x %x  %q\n", tag>>3, l, p[i:i+l])

			i += l
		case tag == Literal:
			w.b = hfmt.Appendf(w.b, "lit  %4x        %q\n", l, p[i:i+l])

			i += l
			w.d.pos += int64(l)
		case tag == Copy:
			var off int

			off, i, err = w.d.roff(p, i)
			if err != nil {
				return st, err
			}

			w.d.pos += int64(l)

			w.b = hfmt.Appendf(w.b, "copy %4x  off %4x (%4x)\n", l, off, off+l)

		//	off += l
		default:
			panic(tag)
		}
	}

	w.GlobalOffset += int64(i)

	if w.Writer != nil {
		_, err = w.Writer.Write(w.b)
	}

	return i, err
}
