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
	tp           Type
	MaxRecordLen int
	err          error

	l *tlog.Logger
}

func NewProtoReader(r io.Reader) *ProtoReader {
	return &ProtoReader{
		r:            r,
		buf:          make([]byte, 0, 1000),
		MaxRecordLen: 16 << 20, // 16MiB
	}
}

func (r *ProtoReader) Type() (Type, error) {
	if r.err != nil {
		return 0, r.wraperr(r.err)
	}

	start := r.pos + r.i
	r.lim = start + 11

again:
	rl, err := r.varint() // record len
	if err != nil {
		return 0, r.wraperr(err)
	}

	if rl == 0 {
		goto again
	}

	err = r.more(rl)
	if err != nil {
		r.l.Printf("err: %+v", err)
		return 0, r.wraperr(err)
	}

	r.lim = r.pos + r.i + rl

	tag := r.buf[r.i]
	r.i++

	ml, err := r.varint() // message len
	if err != nil {
		return 0, r.wraperr(r.err)
	}

	if r.pos+r.i+ml != r.lim {
		r.i = start - r.pos
		return 0, r.newerr("bad length")
	}
	if tag&7 != 2 {
		r.i = start - r.pos
		return 0, r.newerr("bad record type")
	}

	if r.l.V("tag") != nil {
		r.l.Printf("record tag: %x type %x len %x", tag>>3, tag&7, rl)
	}

	switch tag >> 3 {
	case 1:
		r.tp = 'L'
	case 2:
		r.tp = 'l'
	case 3:
		r.tp = 'm'
	case 4:
		r.tp = 's'
	case 5:
		r.tp = 'f'
	default:
		return 0, r.wraperr(fmt.Errorf("unexpected object %x", tag))
	}

	return r.tp, nil
}

func (r *ProtoReader) Any() (interface{}, error) {
	if r.err != nil {
		return nil, r.wraperr(r.err)
	}

	switch r.tp {
	case 'L':
		return r.Labels()
	case 'l':
		return r.Location()
	case 'm':
		return r.Message()
	case 's':
		return r.SpanStart()
	case 'f':
		return r.SpanFinish()
	default:
		return nil, r.wraperr(fmt.Errorf("unexpected object %v", r.tp))
	}
}

func (r *ProtoReader) Read() (interface{}, error) {
	_, _ = r.Type()
	return r.Any()
}

func (r *ProtoReader) Labels() (ls Labels, err error) {
	for r.pos+r.i < r.lim {
		tag := r.buf[r.i]
		r.i++
		if r.l.V("tag") != nil {
			r.l.Printf("tag: %x type %x at %x+%x", tag>>3, tag&7, r.pos, r.i)
		}
		switch tag {
		case 1<<3 | 2:
			x := int(r.buf[r.i])
			r.i++ // len
			copy(ls.Span[:], r.buf[r.i:r.i+x])
			r.i += x
		case 2<<3 | 2:
			l, err := r.string()
			if err != nil {
				return Labels{}, err
			}
			ls.Labels = append(ls.Labels, l)
		default:
			if err = r.skip(); err != nil { //nolint:gocritic
				return Labels{}, err
			}
		}
	}

	if r.l.V("record") != nil {
		r.l.Printf("labels: %q", ls)
	}

	return ls, nil
}

func (r *ProtoReader) Location() (l Location, err error) {
	for r.pos+r.i < r.lim {
		tag := r.buf[r.i]
		r.i++
		if r.l.V("tag") != nil {
			r.l.Printf("tag: %x type %x at %x+%x", tag>>3, tag&7, r.pos, r.i)
		}
		switch tag {
		case 1<<3 | 0:
			x, err := r.varint()
			if err != nil {
				return l, err
			}
			l.PC = uintptr(x)
		case 2<<3 | 0:
			x, err := r.varint()
			if err != nil {
				return l, err
			}
			l.Entry = uintptr(x)
		case 3<<3 | 2:
			l.Name, err = r.string()
			if err != nil {
				return l, err
			}
		case 4<<3 | 2:
			l.File, err = r.string()
			if err != nil {
				return l, err
			}
		case 5<<3 | 0:
			x, err := r.varint()
			if err != nil {
				return l, err
			}
			l.Line = x
		default:
			if err = r.skip(); err != nil { //nolint:gocritic
				return l, err
			}
		}
	}

	if r.l.V("record") != nil {
		r.l.Printf("location: %v", l)
	}

	return l, nil
}

func (r *ProtoReader) Message() (m Message, err error) {
	for r.pos+r.i < r.lim {
		tag := r.buf[r.i]
		r.i++
		if r.l.V("tag") != nil {
			r.l.Printf("tag: %x type %x at %x+%x", tag>>3, tag&7, r.pos, r.i)
		}
		switch tag {
		case 1<<3 | 2:
			x := int(r.buf[r.i])
			r.i++ // len
			copy(m.Span[:], r.buf[r.i:r.i+x])
			r.i += x
		case 2<<3 | 0:
			x, err := r.varint()
			if err != nil {
				return m, err
			}
			m.Location = uintptr(x)
		case 3<<3 | 0:
			x, err := r.varint64()
			if err != nil {
				return m, err
			}
			m.Time = time.Duration(x) << tlog.TimeReduction
		case 4<<3 | 2:
			m.Text, err = r.string()
			if err != nil {
				return m, err
			}
		default:
			if err = r.skip(); err != nil { //nolint:gocritic
				return m, err
			}
		}
	}

	if r.l.V("record") != nil {
		r.l.Printf("message: %v", m)
	}

	return m, nil
}

func (r *ProtoReader) SpanStart() (s SpanStart, err error) {
	for r.pos+r.i < r.lim {
		tag := r.buf[r.i]
		r.i++
		if r.l.V("tag") != nil {
			r.l.Printf("tag: %x type %x at %x+%x", tag>>3, tag&7, r.pos, r.i)
		}
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
				return s, err
			}
			s.Location = uintptr(x)
		case 4<<3 | 0:
			x, err := r.varint64()
			if err != nil {
				return s, err
			}
			s.Started = time.Unix(0, x<<tlog.TimeReduction)
		default:
			if err = r.skip(); err != nil { //nolint:gocritic
				return s, err
			}
		}
	}

	if r.l.V("record") != nil {
		r.l.Printf("span start: %v", s)
	}

	return s, nil
}

func (r *ProtoReader) SpanFinish() (f SpanFinish, err error) {
	for r.pos+r.i < r.lim {
		tag := r.buf[r.i]
		r.i++
		if r.l.V("tag") != nil {
			r.l.Printf("tag: %x type %x at %x+%x", tag>>3, tag&7, r.pos, r.i)
		}
		switch tag {
		case 1<<3 | 2:
			x := int(r.buf[r.i])
			r.i++ // len
			copy(f.ID[:], r.buf[r.i:r.i+x])
			r.i += x
		case 2<<3 | 0:
			x, err := r.varint64()
			if err != nil {
				return f, err
			}
			f.Elapsed = time.Duration(x) << tlog.TimeReduction
		default:
			if err = r.skip(); err != nil { //nolint:gocritic
				return f, err
			}
		}
	}

	if r.l.V("record") != nil {
		r.l.Printf("span finish: %v", f)
	}

	return f, nil
}

func (r *ProtoReader) skip() error {
	tag := r.buf[r.i-1]
	if r.l.V("skip") != nil {
		r.l.Printf("tag: %x type %x unknown tag, skip it", tag>>3, tag&7)
	}

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
	if r.i+x > r.lim {
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
		//	r.l.Printf("c at %x+%x : %x", r.pos, r.i, c)
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
	if r.l.V("") != nil {
		r.l.PrintfDepth(1, "more %3x before pos %3x + %3x buf %3x (%3x) %q", s, r.pos, r.i, len(r.buf), len(r.buf)-r.i, r.buf)
	}
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

again:
	n, err := r.r.Read(r.buf[end:])
	if r.l.V("") != nil {
		r.l.Printf("Read %v %v of %v", n, err, len(r.buf)-end)
	}
	if n != 0 && end+n < s && err == nil {
		end += n
		goto again
	}
	r.buf = r.buf[:end+n]
	if err == io.EOF {
		switch {
		case s <= len(r.buf): // we read all we wanted
			err = nil
		case n == 0: // it's really EOF
		default: // we've expected more data
			err = io.ErrUnexpectedEOF
		}
	}

	if r.l.V("") != nil {
		r.l.PrintfDepth(1, "more %3x after  pos %3x + %3x buf %3x (%3x) %q err %v", s, r.pos, r.i, len(r.buf), len(r.buf)-r.i, r.buf, err)
	}

	return err
}

func (r *ProtoReader) newerr(msg string) error {
	if r.err != nil {
		return r.err
	}

	r.err = fmt.Errorf(msg+" (pos: %d)", r.pos+r.i)
	return r.err
}

func (r *ProtoReader) wraperr(err error) error {
	if r.err != nil {
		return r.err
	}
	if err == io.EOF {
		r.err = err
		return err
	}

	r.err = fmt.Errorf("%v (pos: %d)", err, r.pos+r.i)
	return r.err
}
