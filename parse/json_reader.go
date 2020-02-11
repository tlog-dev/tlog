package parse

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/nikandfor/json"

	"github.com/nikandfor/tlog"
)

type JSONReader struct {
	r *json.Reader
}

func NewJSONReader(r io.Reader) *JSONReader {
	return NewCustomJSONReader(json.NewReader(r))
}

func NewCustomJSONReader(r *json.Reader) *JSONReader {
	return &JSONReader{r: r}
}

func (r *JSONReader) Read() (interface{}, error) {
	if err := r.r.Err(); err != nil {
		return nil, err
	}
	if !r.r.HasNext() {
		return nil, io.EOF
	}
	defer r.r.HasNext()

	tp := r.r.NextString()
	if len(tp) != 1 {
		return nil, r.r.ErrorHere(fmt.Errorf("unexpected object %q", tp))
	}

	switch tp[0] {
	case 'L':
		return r.labels()
	case 'l':
		return r.location()
	case 'm':
		return r.message()
	case 's':
		return r.spanStart()
	case 'f':
		return r.spanFinish()
	default:
		return nil, r.r.ErrorHere(fmt.Errorf("unexpected object %q", tp))
	}
}

func (r *JSONReader) labels() (ls Labels, err error) {
	if r.r.Type() != json.Array {
		return nil, r.r.ErrorHere(errors.New("array expected"))
	}

	for r.r.HasNext() {
		l := string(r.r.NextString())
		ls = append(ls, l)
	}

	return ls, nil
}

func (r *JSONReader) location() (l Location, err error) {
	if r.r.Type() != json.Object {
		return Location{}, r.r.ErrorHere(errors.New("object expected"))
	}

	for r.r.HasNext() {
		k := r.r.NextString()
		if len(k) == 0 {
			return Location{}, r.r.ErrorHere(errors.New("empty key"))
		}
		switch k[0] {
		case 'f':
			l.File = string(r.r.NextString())
		case 'n':
			l.Name = string(r.r.NextString())
		case 'p':
			n := string(r.r.NextNumber())
			v, err := strconv.ParseUint(n, 10, 64)
			if err != nil {
				return Location{}, r.r.ErrorHere(err)
			}
			l.PC = uintptr(v)
		case 'l':
			n := string(r.r.NextNumber())
			v, err := strconv.ParseUint(n, 10, 64)
			if err != nil {
				return Location{}, r.r.ErrorHere(err)
			}
			l.Line = int(v)
		default:
			tlog.V("skip").Printf("skip key %q", k)
			r.r.Skip()
		}
	}

	tlog.V("record").Printf("location: %v", l)

	return l, nil
}

func (r *JSONReader) message() (m Message, err error) {
	if r.r.Type() != json.Object {
		return Message{}, r.r.ErrorHere(errors.New("object expected"))
	}

	for r.r.HasNext() {
		k := r.r.NextString()
		if len(k) == 0 {
			return Message{}, r.r.ErrorHere(errors.New("empty key"))
		}
		switch k[0] {
		case 'm':
			m.Text = string(r.r.NextString())
		case 'l':
			n := string(r.r.NextNumber())
			v, err := strconv.ParseUint(n, 10, 64)
			if err != nil {
				return Message{}, r.r.ErrorHere(err)
			}
			m.Location = uintptr(v)
		case 't':
			n := string(r.r.NextNumber())
			v, err := strconv.ParseInt(n, 10, 64)
			if err != nil {
				return Message{}, r.r.ErrorHere(err)
			}
			m.Time = time.Duration(v << tlog.TimeReduction)
		case 's':
			m.Span, err = r.id()
			if err != nil {
				return Message{}, r.r.ErrorHere(err)
			}
		default:
			tlog.V("skip").Printf("skip key %q", k)
			r.r.Skip()
		}
	}

	tlog.V("record").Printf("message: %v", m)

	return m, nil
}

func (r *JSONReader) spanStart() (s SpanStart, err error) {
	if r.r.Type() != json.Object {
		return SpanStart{}, r.r.ErrorHere(errors.New("object expected"))
	}

	for r.r.HasNext() {
		k := r.r.NextString()
		if len(k) == 0 {
			return SpanStart{}, r.r.ErrorHere(errors.New("empty key"))
		}
		switch k[0] {
		case 'l':
			n := string(r.r.NextNumber())
			v, err := strconv.ParseUint(n, 10, 64)
			if err != nil {
				return SpanStart{}, r.r.ErrorHere(err)
			}
			s.Location = uintptr(v)
		case 's':
			n := string(r.r.NextNumber())
			v, err := strconv.ParseInt(n, 10, 64)
			if err != nil {
				return SpanStart{}, r.r.ErrorHere(err)
			}
			s.Started = time.Unix(0, v<<tlog.TimeReduction)
		case 'i':
			s.ID, err = r.id()
			if err != nil {
				return SpanStart{}, r.r.ErrorHere(err)
			}
		case 'p':
			s.Parent, err = r.id()
			if err != nil {
				return SpanStart{}, r.r.ErrorHere(err)
			}
		default:
			tlog.V("skip").Printf("skip key %q", k)
			r.r.Skip()
		}
	}

	tlog.V("record").Printf("span start: %v", s)

	return s, nil
}

func (r *JSONReader) spanFinish() (f SpanFinish, err error) {
	if r.r.Type() != json.Object {
		return SpanFinish{}, r.r.ErrorHere(errors.New("object expected"))
	}

	for r.r.HasNext() {
		k := r.r.NextString()
		if len(k) == 0 {
			return SpanFinish{}, r.r.ErrorHere(errors.New("empty key"))
		}
		switch k[0] {
		case 'i':
			f.ID, err = r.id()
			if err != nil {
				return SpanFinish{}, r.r.ErrorHere(err)
			}
		case 'e':
			n := string(r.r.NextNumber())
			v, err := strconv.ParseInt(n, 10, 64)
			if err != nil {
				return SpanFinish{}, r.r.ErrorHere(err)
			}
			f.Elapsed = time.Duration(v << tlog.TimeReduction)
		default:
			tlog.V("skip").Printf("skip key %q", k)
			r.r.Skip()
		}
	}

	tlog.V("record").Printf("span finish: %v", f)

	return f, nil
}

func (r *JSONReader) id() (id ID, err error) {
	s := r.r.NextString()
	if len(s) > 2*len(id) {
		return id, errors.New("too big id")
	}
	_, err = hex.Decode(id[:], s)
	if err != nil {
		return id, err
	}
	return
}
