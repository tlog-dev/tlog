package parse

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/nikandfor/tlog"
)

type ProtobufReader struct {
	r   io.Reader
	buf []byte
	i   int
	pos int
}

func NewProtobufReader(r io.Reader) *ProtobufReader {
	return &ProtobufReader{
		r:   r,
		buf: make([]byte, 0, 10),
	}
}

func (r *ProtobufReader) Read() (interface{}, error) {
	rl, err := r.varint() // record len
	if err != nil {
		return nil, err
	}

	err = r.more(rl)
	if err != nil {
		tlog.Printf("err: %v", err)
		return nil, err
	}

	tag := r.buf[r.i]
	r.i++

	tlog.V("tag").Printf("record tag: %v", tag>>3)

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

func (r *ProtobufReader) labels() (interface{}, error) {
	tl, err := r.varint()
	if err != nil {
		return nil, err
	}

	var ls Labels
	for i := r.pos + r.i; r.pos+r.i < i+tl; {
		tag := r.buf[r.i]
		r.i++
		tlog.V("tag").Printf("tag: %x (type %x) at %x+%x", tag>>3, tag&7, r.pos, r.i)
		switch tag {
		case 1<<3 | 2:
			x, err := r.varint()
			if err != nil {
				return nil, err
			}
			l := string(r.buf[r.i : r.i+x])
			r.i += x
			ls = append(ls, l)
		default:
			if err = r.skip(); err != nil {
				return nil, err
			}
		}
	}

	tlog.V("record").Printf("labels: %q", ls)

	return ls, nil
}

func (r *ProtobufReader) location() (interface{}, error) {
	tl, err := r.varint() // total len
	if err != nil {
		return nil, err
	}

	var l Location
	for i := r.pos + r.i; r.pos+r.i < i+tl; {
		tag := r.buf[r.i]
		r.i++
		tlog.V("tag").Printf("tag: %x (type %x) at %x+%x", tag>>3, tag&7, r.pos, r.i)
		switch tag {
		case 1<<3 | 0:
			x, err := r.varint()
			if err != nil {
				return nil, err
			}
			l.PC = uintptr(x)
		case 2<<3 | 2:
			x, err := r.varint()
			if err != nil {
				return nil, err
			}
			l.Name = string(r.buf[r.i : r.i+x])
			r.i += x
		case 3<<3 | 2:
			x, err := r.varint()
			if err != nil {
				return nil, err
			}
			l.File = string(r.buf[r.i : r.i+x])
			r.i += x
		case 4<<3 | 0:
			x, err := r.varint()
			if err != nil {
				return nil, err
			}
			l.Line = x
		default:
			if err = r.skip(); err != nil {
				return nil, err
			}
		}
	}

	tlog.V("record").Printf("location: %v", l)

	return l, nil
}

func (r *ProtobufReader) message() (interface{}, error) {
	tl, err := r.varint() // total len
	if err != nil {
		return nil, err
	}

	var m Message
	for i := r.pos + r.i; r.pos+r.i < i+tl; {
		tag := r.buf[r.i]
		r.i++
		tlog.V("tag").Printf("tag: %x (type %x) at %x+%x", tag>>3, tag&7, r.pos, r.i)
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
			x, err := r.varint()
			if err != nil {
				return nil, err
			}
			m.Text = string(r.buf[r.i : r.i+x])
			r.i += x
		default:
			if err = r.skip(); err != nil {
				return nil, err
			}
		}
	}

	tlog.V("record").Printf("message: %v", m)

	return m, nil
}

func (r *ProtobufReader) spanStart() (interface{}, error) {
	tl, err := r.varint() // total len
	if err != nil {
		return nil, err
	}

	var s SpanStart
	for i := r.pos + r.i; r.pos+r.i < i+tl; {
		tag := r.buf[r.i]
		r.i++
		tlog.V("tag").Printf("tag: %x (type %x) at %x+%x", tag>>3, tag&7, r.pos, r.i)
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
			s.Started = time.Unix(0, int64(x)<<tlog.TimeReduction)
		default:
			if err = r.skip(); err != nil {
				return nil, err
			}
		}
	}

	tlog.V("record").Printf("span start: %v", s)

	return s, nil
}

func (r *ProtobufReader) spanFinish() (interface{}, error) {
	tl, err := r.varint() // total len
	if err != nil {
		return nil, err
	}

	var f SpanFinish
	for i := r.pos + r.i; r.pos+r.i < i+tl; {
		tag := r.buf[r.i]
		r.i++
		tlog.V("tag").Printf("tag: %x (type %x) at %x+%x", tag>>3, tag&7, r.pos, r.i)
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
			if err = r.skip(); err != nil {
				return nil, err
			}
		}
	}

	tlog.V("record").Printf("span finish: %v", f)

	return f, nil
}

func (r *ProtobufReader) skip() error {
	tag := r.buf[r.i-1]
	tlog.V("skip").Printf("unknown tag found: %x type %x", tag>>3, tag&7)

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
		r.i += x
	default:
		return fmt.Errorf("unsupported tag type: %v", tag&7)
	}

	return nil
}

func (r *ProtobufReader) varint() (x int, err error) {
	s := uint(0)
	for i := 0; ; i++ {
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
				return x, errors.New("varint overflow")
			}
			return x | int(c)<<s, nil
		}
		x |= int(c&0x7f) << s
		s += 7
	}
}

func (r *ProtobufReader) varint64() (x int64, err error) {
	s := uint(0)
	for i := 0; ; i++ {
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
				return x, errors.New("varint overflow")
			}
			return x | int64(c)<<s, nil
		}
		x |= int64(c&0x7f) << s
		s += 7
	}
}

func (r *ProtobufReader) more(s int) error {
	r.pos += r.i
	end := 0
	if r.i < len(r.buf) {
		copy(r.buf, r.buf[r.i:])
		end = len(r.buf) - r.i
	}
	r.i = 0

	for cap(r.buf) < s {
		r.buf = append(r.buf, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	}
	r.buf = r.buf[:cap(r.buf)]

	n, err := r.r.Read(r.buf[end:])
	r.buf = r.buf[:end+n]
	if err == io.EOF {
		if r.i+s <= len(r.buf) {
			err = nil
		}
	}

	return err
}

func (r *ProtobufReader) newerr(msg string, args ...interface{}) error {
	return fmt.Errorf(msg+" at pos %d", append(args, r.pos+r.i)...)
}
