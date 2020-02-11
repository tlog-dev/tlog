package parse

import (
	"fmt"
	"io"
	"time"

	"github.com/nikandfor/tlog"
)

type ProtoReader struct {
	r            io.Reader
	buf          []byte
	i            int
	pos          int
	lim          int
	MaxRecordLen int
}

func NewProtoReader(r io.Reader) *ProtoReader {
	return &ProtoReader{
		r:            r,
		buf:          make([]byte, 0, 10),
		MaxRecordLen: 16 << 20, // 16MiB
	}
}

func (r *ProtoReader) Read() (interface{}, error) {
	start := r.pos + r.i
	r.lim = start + 11

again:
	rl, err := r.varint() // record len
	if err != nil {
		return nil, err
	}

	if rl == 0 {
		goto again
	}

	err = r.more(rl)
	if err != nil {
		tlog.Printf("err: %v", err)
		return nil, err
	}

	r.lim = r.pos + r.i + rl

	tag := r.buf[r.i]
	r.i++

	ml, err := r.varint() // message len
	if err != nil {
		return nil, err
	}

	if r.pos+r.i+ml != r.lim {
		r.i = start - r.pos
		return nil, r.newerr("bad length")
	}
	if tag&7 != 2 {
		r.i = start - r.pos
		return nil, r.newerr("bad record type")
	}

	tlog.V("tag").Printf("record tag: %x type %x len %x", tag>>3, tag&7, rl)

	switch tag >> 3 {
	case 1:
		return r.labels()
	case 2:
		return r.location()
	case 3:
		return r.message()
	case 4:
		return r.spanStart()
	case 5:
		return r.spanFinish()
	default:
		return nil, fmt.Errorf("unexpected object %x", tag)
	}
}

func (r *ProtoReader) labels() (_ interface{}, err error) {
	var ls Labels

	for r.pos+r.i < r.lim {
		tag := r.buf[r.i]
		r.i++
		tlog.V("tag").Printf("tag: %x type %x at %x+%x", tag>>3, tag&7, r.pos, r.i)
		switch tag {
		case 1<<3 | 2:
			l, err := r.string()
			if err != nil {
				return nil, err
			}
			ls = append(ls, l)
		default:
			if err = r.skip(); err != nil { //nolint:gocritic
				return nil, err
			}
		}
	}

	tlog.V("record").Printf("labels: %q", ls)

	return ls, nil
}

func (r *ProtoReader) location() (_ interface{}, err error) {
	var l Location

	for r.pos+r.i < r.lim {
		tag := r.buf[r.i]
		r.i++
		tlog.V("tag").Printf("tag: %x type %x at %x+%x", tag>>3, tag&7, r.pos, r.i)
		switch tag {
		case 1<<3 | 0:
			x, err := r.varint()
			if err != nil {
				return nil, err
			}
			l.PC = uintptr(x)
		case 2<<3 | 2:
			l.Name, err = r.string()
			if err != nil {
				return nil, err
			}
		case 3<<3 | 2:
			l.File, err = r.string()
			if err != nil {
				return nil, err
			}
		case 4<<3 | 0:
			x, err := r.varint()
			if err != nil {
				return nil, err
			}
			l.Line = x
		default:
			if err = r.skip(); err != nil { //nolint:gocritic
				return nil, err
			}
		}
	}

	tlog.V("record").Printf("location: %v", l)

	return l, nil
}

func (r *ProtoReader) message() (_ interface{}, err error) {
	var m Message

	for r.pos+r.i < r.lim {
		tag := r.buf[r.i]
		r.i++
		tlog.V("tag").Printf("tag: %x type %x at %x+%x", tag>>3, tag&7, r.pos, r.i)
		switch tag {
		case 1<<3 | 2:
			x := int(r.buf[r.i])
			r.i++ // len
			copy(m.Span[:], r.buf[r.i:r.i+x])
			r.i += x
		case 2<<3 | 0:
			x, err := r.varint()
			if err != nil {
				return nil, err
			}
			m.Location = uintptr(x)
		case 3<<3 | 0:
			x, err := r.varint64()
			if err != nil {
				return nil, err
			}
			m.Time = time.Duration(x) << tlog.TimeReduction
		case 4<<3 | 2:
			m.Text, err = r.string()
			if err != nil {
				return nil, err
			}
		default:
			if err = r.skip(); err != nil { //nolint:gocritic
				return nil, err
			}
		}
	}

	tlog.V("record").Printf("message: %v", m)

	return m, nil
}

func (r *ProtoReader) spanStart() (_ interface{}, err error) {
	var s SpanStart

	for r.pos+r.i < r.lim {
		tag := r.buf[r.i]
		r.i++
		tlog.V("tag").Printf("tag: %x type %x at %x+%x", tag>>3, tag&7, r.pos, r.i)
		switch tag {
		case 1<<3 | 2:
			x := int(r.buf[r.i])
			r.i++ // len
			copy(s.ID[:], r.buf[r.i:r.i+x])
			r.i += x
		case 2<<3 | 2:
			x := int(r.buf[r.i])
			r.i++ // len
			copy(s.Parent[:], r.buf[r.i:r.i+x])
			r.i += x
		case 3<<3 | 0:
			x, err := r.varint()
			if err != nil {
				return nil, err
			}
			s.Location = uintptr(x)
		case 4<<3 | 0:
			x, err := r.varint64()
			if err != nil {
				return nil, err
			}
			s.Started = time.Unix(0, x<<tlog.TimeReduction)
		default:
			if err = r.skip(); err != nil { //nolint:gocritic
				return nil, err
			}
		}
	}

	tlog.V("record").Printf("span start: %v", s)

	return s, nil
}

func (r *ProtoReader) spanFinish() (_ interface{}, err error) {
	var f SpanFinish

	for r.pos+r.i < r.lim {
		tag := r.buf[r.i]
		r.i++
		tlog.V("tag").Printf("tag: %x type %x at %x+%x", tag>>3, tag&7, r.pos, r.i)
		switch tag {
		case 1<<3 | 2:
			x := int(r.buf[r.i])
			r.i++ // len
			copy(f.ID[:], r.buf[r.i:r.i+x])
			r.i += x
		case 2<<3 | 0:
			x, err := r.varint64()
			if err != nil {
				return nil, err
			}
			f.Elapsed = time.Duration(x) << tlog.TimeReduction
		default:
			if err = r.skip(); err != nil { //nolint:gocritic
				return nil, err
			}
		}
	}

	tlog.V("record").Printf("span finish: %v", f)

	return f, nil
}

func (r *ProtoReader) skip() error {
	tag := r.buf[r.i-1]
	tlog.V("skip").Printf("tag: %x type %x unknown tag, skip it", tag>>3, tag&7)

	switch tag & 7 {
	case 0:
		_, err := r.varint()
		if err != nil {
			return err
		}
	case 2:
		x, err := r.varint()
		if err != nil {
			return err
		}
		err = r.more(x)
		if err != nil {
			return err
		}
		r.i += x
	default:
		return fmt.Errorf("unsupported tag type: %v", tag&7)
	}

	return nil
}

func (r *ProtoReader) string() (s string, err error) {
	i := r.i
	x, err := r.varint()
	if err != nil {
		return "", err
	}
	if r.i+x > len(r.buf) {
		r.i = i
		return "", r.newerr("out of string")
	}
	s = string(r.buf[r.i : r.i+x])
	r.i += x
	return
}

func (r *ProtoReader) varint() (int, error) {
	x, err := r.varint64()
	return int(x), err
}

func (r *ProtoReader) varint64() (x int64, err error) {
	s := uint(0)
	for i := 0; ; i++ {
		if r.pos+r.i == r.lim {
			return 0, r.wraperr(io.ErrUnexpectedEOF)
		}
		if r.i == len(r.buf) {
			if err = r.more(1); err != nil {
				return
			}
		}
		c := r.buf[r.i]
		//	tlog.Printf("c at %x+%x : %x", r.pos, r.i, c)
		r.i++

		if c < 0x80 {
			if i > 9 || i == 9 && c > 1 {
				r.i -= i // to have position on start of varint
				return x, r.newerr("varint overflow")
			}
			return x | int64(c)<<s, nil
		}
		x |= int64(c&0x7f) << s
		s += 7
	}
}

func (r *ProtoReader) more(s int) error {
	tlog.V("").Printf("more %3x before pos %3x + %3x buf %3x (%3x) %q", s, r.pos, r.i, len(r.buf), len(r.buf)-r.i, r.buf)
	r.pos += r.i
	end := 0
	if r.i < len(r.buf) {
		copy(r.buf, r.buf[r.i:])
		end = len(r.buf) - r.i
	}
	r.i = 0

	for cap(r.buf) < s {
		if s >= r.MaxRecordLen {
			return r.newerr("too big record")
		}
		r.buf = append(r.buf, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
		r.buf = r.buf[:cap(r.buf)]
	}
	r.buf = r.buf[:cap(r.buf)]

	n, err := r.r.Read(r.buf[end:])
	r.buf = r.buf[:end+n]
	if err == io.EOF {
		if r.i+s <= len(r.buf) {
			err = nil
		}
	}

	tlog.V("").Printf("more %3x after  pos %3x + %3x buf %3x (%3x) %q", s, r.pos, r.i, len(r.buf), len(r.buf)-r.i, r.buf)

	return err
}

func (r *ProtoReader) newerr(msg string) error {
	return fmt.Errorf(msg+" (pos: %d)", r.pos+r.i)
}

func (r *ProtoReader) wraperr(err error) error {
	return fmt.Errorf("%v (pos: %d)", err, r.pos+r.i)
}
