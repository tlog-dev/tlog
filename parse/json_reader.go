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
	r      *json.Reader
	tp     Type
	finish bool
	err    error
}

func NewJSONReader(r io.Reader) *JSONReader {
	return NewCustomJSONReader(json.NewReader(r))
}

func NewCustomJSONReader(r *json.Reader) *JSONReader {
	return &JSONReader{r: r}
}

func (r *JSONReader) Type() (Type, error) {
	if r.err != nil {
		return 0, r.err
	}

	if r.finish {
		if r.r.HasNext() {
			return 0, r.wraperr(fmt.Errorf("expected end of object, got %v", r.r.Type()))
		}
	}
	if !r.r.HasNext() {
		return 0, r.wraperr(io.EOF)
	}
	r.finish = true

	tp := r.r.NextString()
	if len(tp) != 1 {
		return 0, r.wraperr(fmt.Errorf("unexpected object %q", tp))
	}

	tlog.V("tag").Printf("record tag: %v type %v", Type(tp[0]), r.r.Type())

	switch tp[0] {
	case 'L', 'l', 'm', 's', 'f':
		r.tp = Type(tp[0])
		return r.tp, nil
	default:
		r.tp = 0
		return 0, r.wraperr(fmt.Errorf("unexpected object %q", tp))
	}
}

func (r *JSONReader) Any() (interface{}, error) {
	if r.err != nil {
		return 0, r.err
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
		return nil, r.r.ErrorHere(fmt.Errorf("unexpected object %q", r.tp))
	}
}

func (r *JSONReader) Read() (interface{}, error) {
	_, _ = r.Type()
	return r.Any()
}

func (r *JSONReader) Labels() (ls Labels, err error) {
	if r.r.Type() != json.Array {
		return nil, r.r.ErrorHere(fmt.Errorf("array expected, got %v %v", r.r.Type(), r.tp))
	}

	for r.r.HasNext() {
		l := string(r.r.NextString())
		ls = append(ls, l)
	}

	tlog.V("record").Printf("labels: %v", ls)

	return ls, nil
}

func (r *JSONReader) Location() (l Location, err error) {
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

func (r *JSONReader) Message() (m Message, err error) {
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

func (r *JSONReader) SpanStart() (s SpanStart, err error) {
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

func (r *JSONReader) SpanFinish() (f SpanFinish, err error) {
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

func (r *JSONReader) wraperr(err error) error {
	if r.err != nil {
		return r.err
	}
	if err == io.EOF {
		r.err = err
		return err
	}
	r.err = r.r.ErrorHere(err)
	return r.err
}
