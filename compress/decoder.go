package compress

import (
	"fmt"
	"io"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/low"
)

type (
	Decoder struct {
		io.Reader

		block []byte
		pos   int
		mask  int

		state    byte
		off, len int

		b           []byte
		ref, i, end int

		err error
	}

	Dumper struct {
		io.Writer

		d Decoder

		NoGlobalOffset bool

		ref int
		b   low.Buf
	}
)

func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{
		Reader: r,
	}
}

func NewDecoderBytes(b []byte) *Decoder {
	return &Decoder{
		b:   b,
		end: len(b),
	}
}

func (r *Decoder) Reset(rd io.Reader) {
	r.ResetBytes(nil)
	r.Reader = rd
}

func (r *Decoder) ResetBytes(b []byte) {
	r.Reader = nil

	if b != nil {
		r.b = b
	}
	r.i = 0
	r.end = len(b)
	r.ref = 0

	r.state = 0
	r.err = nil
}

func (r *Decoder) Read(p []byte) (i int, err error) {
	if r.err != nil {
		return 0, r.err
	}

more:
	switch r.state {
	case 0:
		tag, l := r.tag()
		if r.err != nil {
			return i, r.err
		}

		switch tag {
		case Literal:
			tl.Printw("tag", "name", "literal", "tag", tlog.Hex(tag), "len", tlog.Hex(l))

			r.state = 'l'
			r.len = l
		case Copy:
			r.off = r.readOff()
			if r.err != nil {
				return i, r.err
			}

			r.off = r.pos - r.off - l

			tl.Printw("tag", "name", "copy", "tag", tlog.Hex(tag), "len", tlog.Hex(l), "off", tlog.Hex(r.off))

			r.state = 'c'
			r.len = l
		case Meta:
			switch l {
			case MetaReset:
				bslog := r.readOff()
				if r.err != nil {
					return i, r.err
				}

				r.block = make([]byte, 1<<bslog)
				r.pos = 0
				r.mask = 1<<bslog - 1

				r.state = 0

				tl.Printw("tag", "name", "meta", "tag", tlog.Hex(tag), "sub", tlog.Hex(l), "sub_name", "block_size", "block_size", len(r.block))
			default:
				return i, r.newErr("unsupported meta tag: %x", l)
			}
		default:
			return i, r.newErr("impossible tag: %x", tag)
		}
	case 'l':
		end := len(p)
		if end > i+r.len {
			end = i + r.len
		}

		if !r.more(end - i) {
			return i, r.err
		}

		tl.Printw("literal", "i", tlog.Hex(i), "end", tlog.Hex(end), "r.i", tlog.Hex(r.i), "r.pos", tlog.Hex(r.pos))

		n := copy(p[i:end], r.b[r.i:])
		i += n
		r.len -= n

		end = r.i + n
		for r.i < end {
			m := copy(r.block[r.pos&r.mask:], r.b[r.i:end])
			r.i += m
			r.pos += m
		}
	case 'c':
		end := len(p)
		if end > i+r.len {
			end = i + r.len
		}

		tl.Printw("copy", "i", tlog.Hex(i), "end", tlog.Hex(end), "r.off", tlog.Hex(r.off), "r.pos", tlog.Hex(r.pos))

		n := copy(p[i:end], r.block[r.off:])
		r.off += n
		r.len -= n

		end = i + n
		for i < end {
			m := copy(r.block[r.pos&r.mask:], p[i:end])
			i += m
			r.pos += m
		}
	}

	if r.len == 0 {
		r.state = 0
	}

	if i < len(p) {
		goto more
	}

	return i, r.err
}

func (r *Decoder) newErr(f string, args ...interface{}) error {
	if r.err == nil {
		r.err = fmt.Errorf(f, args...)
	}

	return r.err
}

func (r *Decoder) readOff() (l int) {
	if !r.more(1) {
		return
	}

	l = int(r.b[r.i])
	r.i++

	switch l {
	case Off1:
		if !r.more(1) {
			return
		}

		l = int(r.b[r.i])
		r.i++
	case Off2:
		if !r.more(2) {
			return
		}

		l = int(r.b[r.i])<<8 | int(r.b[r.i+1])
		r.i += 2
	case Off4:
		if !r.more(4) {
			return
		}

		l = int(r.b[r.i])<<24 | int(r.b[r.i+1])<<16 | int(r.b[r.i+2])<<8 | int(r.b[r.i+3])
		r.i += 4
	case Off8:
		if !r.more(8) {
			return
		}

		l = int(r.b[r.i])<<56 | int(r.b[r.i+1])<<48 | int(r.b[r.i+2])<<40 | int(r.b[r.i+3])<<32 |
			int(r.b[r.i+4])<<24 | int(r.b[r.i+5])<<16 | int(r.b[r.i+6])<<8 | int(r.b[r.i+7])
		r.i += 8
	}

	return
}

func (r *Decoder) tag() (tag, l int) {
	if !r.more(1) {
		return
	}

	tag = int(r.b[r.i]) & TagMask
	l = int(r.b[r.i]) & TagLenMask
	r.i++

	switch l {
	case TagLen1:
		if !r.more(1) {
			return
		}

		l = int(r.b[r.i])
		r.i++
	case TagLen2:
		if !r.more(2) {
			return
		}

		l = int(r.b[r.i])<<8 | int(r.b[r.i+1])
		r.i += 2
	case TagLen4:
		if !r.more(4) {
			return
		}

		l = int(r.b[r.i])<<24 | int(r.b[r.i+1])<<16 | int(r.b[r.i+2])<<8 | int(r.b[r.i+3])
		r.i += 4
	case TagLen8:
		if !r.more(8) {
			return
		}

		l = int(r.b[r.i])<<56 | int(r.b[r.i+1])<<48 | int(r.b[r.i+2])<<40 | int(r.b[r.i+3])<<32 |
			int(r.b[r.i+4])<<24 | int(r.b[r.i+5])<<16 | int(r.b[r.i+6])<<8 | int(r.b[r.i+7])
		r.i += 8
	}

	return
}

func (r *Decoder) more(l int) bool {
	if r.err != nil {
		return false
	}

	if r.i+l <= r.end {
		return true
	}

	tl.Printw("more", "r.i", r.i, "r.end", r.end, "len", l)

	if r.Reader == nil {
		if r.i == r.end {
			r.err = io.EOF
		} else {
			r.err = io.ErrUnexpectedEOF
		}

		return false
	}

	panic("implement")
}

func NewDumper(w io.Writer) *Dumper {
	return &Dumper{
		Writer: w,
	}
}

func (w *Dumper) Write(p []byte) (n int, err error) {
	w.d.ResetBytes(p)
	w.b = w.b[:0]

	for w.d.i < w.d.end {
		if !w.NoGlobalOffset {
			w.b = low.AppendPrintf(w.b, "%8x  ", w.ref+w.d.i)
		}

		st := w.d.i

		tag, l := w.d.tag()
		if w.d.err != nil {
			return w.d.i, w.d.err
		}

		switch tag {
		case Literal:
			w.b = low.AppendPrintf(w.b, "%4x  %4x literal        %q\n", st, l, p[w.d.i:w.d.i+l])
			w.d.i += l
		case Copy:
			off := w.d.readOff()
			off += l

			w.b = low.AppendPrintf(w.b, "%4x  %4x copy off %4x\n", st, l, off)
		case Meta:
			arg := w.d.readOff()
			w.b = low.AppendPrintf(w.b, "%4x  %4x meta %x\n", st, 2, arg)
		default:
			return w.d.i, w.d.newErr("impossible tag: %x", tag)
		}
	}

	if w.d.err != nil {
		return 0, w.d.err
	}

	w.ref += w.d.i

	if w.Writer != nil {
		return w.Writer.Write(w.b)
	}

	return w.d.i, nil
}
