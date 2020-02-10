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
	tlog.Printf("start reading record at %x (%x+%x)  buf len %x", r.pos+r.i, r.pos, r.i, len(r.buf))
	l, err := r.varint() // record len
	if err != nil {
		return nil, err
	}
	tlog.Printf("record len: %x  at %x", l, r.pos+r.i)

	defer func(st int) {
		if st+l != r.pos+r.i {
			tlog.Printf("wrong end of record: expected %x (%x+%x) got %x", st+l, st, l, r.pos+r.i)
		}
	}(r.pos + r.i)

	err = r.more(l)
	if err != nil {
		tlog.Printf("err: %v", err)
		return nil, err
	}

	tag := r.buf[r.i]
	r.i++

	tlog.Printf("proto tag: %v", tag>>3)

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

	tlog.Printf("labels len %x", tl)

	var ls Labels

	for i := r.pos + r.i; r.pos+r.i < i+tl; {
		r.i++ // tag
		l, err := r.varint()
		if err != nil {
			return nil, err
		}

		tlog.Printf("label: %q", r.buf[r.i:r.i+l])

		ls = append(ls, string(r.buf[r.i:r.i+l]))
		r.i += l
	}

	return ls, nil
}

func (r *ProtobufReader) location() (interface{}, error) {
	_, err := r.varint() // total len
	if err != nil {
		return nil, err
	}

	var l Location

	tlog.Printf("tag: %x at %x+%x", r.buf[r.i]>>3, r.pos, r.i)
	r.i++ // tag
	x, err := r.varint()
	if err != nil {
		return nil, err
	}
	l.PC = uintptr(x)

	tlog.Printf("tag: %x at %x+%x", r.buf[r.i]>>3, r.pos, r.i)
	r.i++ // tag
	x, err = r.varint()
	if err != nil {
		return nil, err
	}
	l.Name = string(r.buf[r.i : r.i+x])
	r.i += x

	tlog.Printf("tag: %x at %x+%x", r.buf[r.i]>>3, r.pos, r.i)
	r.i++ // tag
	x, err = r.varint()
	if err != nil {
		return nil, err
	}
	l.File = string(r.buf[r.i : r.i+x])
	r.i += x

	tlog.Printf("tag: %x at %x+%x", r.buf[r.i]>>3, r.pos, r.i)
	r.i++ // tag
	x, err = r.varint()
	if err != nil {
		return nil, err
	}
	l.Line = x

	tlog.Printf("location: %v", l)

	return l, nil
}

func (r *ProtobufReader) message() (interface{}, error) {
	total, err := r.varint() // total len
	if err != nil {
		return nil, err
	}
	tlog.Printf("record len: %x", total)

	var m Message

	if r.buf[r.i] == 1<<3|2 {
		r.i++ // tag
		r.i++ // len
		copy(m.Span[:], r.buf[r.i:])
		r.i += len(m.Span)
	}

	tlog.Printf("tag: %x at %x+%x", r.buf[r.i]>>3, r.pos, r.i)
	if r.buf[r.i] == 2<<3|0 {
		r.i++ // tag
		x, err := r.varint()
		if err != nil {
			return nil, err
		}
		m.Location = uintptr(x)
	}

	tlog.Printf("tag: %x at %x+%x", r.buf[r.i]>>3, r.pos, r.i)
	r.i++ // tag
	x, err := r.varint()
	if err != nil {
		return nil, err
	}
	m.Time = time.Duration(x) << tlog.TimeReduction

	tlog.Printf("tag: %x at %x+%x", r.buf[r.i]>>3, r.pos, r.i)
	r.i++ // tag
	x, err = r.varint()
	tlog.Printf("msg text: %x+%x  %x", r.pos, r.i, x)
	if err != nil {
		return nil, err
	}
	m.Text = string(r.buf[r.i : r.i+x])
	r.i += x

	tlog.Printf("message: %v", m)

	return m, nil
}

func (r *ProtobufReader) spanStart() (interface{}, error) {
	_, err := r.varint() // total len
	if err != nil {
		return nil, err
	}

	var s SpanStart

	tlog.Printf("tag: %x at %x+%x", r.buf[r.i]>>3, r.pos, r.i)
	r.i++ //tag
	r.i++ // len
	copy(s.ID[:], r.buf[r.i:])
	r.i += len(s.ID)

	tlog.Printf("tag: %x at %x+%x", r.buf[r.i]>>3, r.pos, r.i)
	if r.buf[r.i] == 2<<3|2 {
		r.i++ //tag
		r.i++ // len
		copy(s.Parent[:], r.buf[r.i:])
		r.i += len(s.ID)
	}

	tlog.Printf("tag: %x at %x+%x", r.buf[r.i]>>3, r.pos, r.i)
	if r.buf[r.i] == 3<<3|0 {
		r.i++ // tag
		x, err := r.varint()
		if err != nil {
			return nil, err
		}
		s.Location = uintptr(x)
	}

	tlog.Printf("tag: %x at %x+%x", r.buf[r.i]>>3, r.pos, r.i)
	r.i++ // tag
	x, err := r.varint()
	if err != nil {
		return nil, err
	}
	s.Started = time.Unix(0, int64(x)<<tlog.TimeReduction)

	tlog.Printf("span start: %v", s)

	return s, nil
}

func (r *ProtobufReader) spanFinish() (interface{}, error) {
	_, err := r.varint() // total len
	if err != nil {
		return nil, err
	}

	var f SpanFinish

	r.i++ //tag
	r.i++ // len
	copy(f.ID[:], r.buf[r.i:])
	r.i += len(f.ID)

	r.i++ // tag
	x, err := r.varint()
	if err != nil {
		return nil, err
	}
	f.Elapsed = time.Duration(x) << tlog.TimeReduction

	return f, nil
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
