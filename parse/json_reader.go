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
		return r.span()
	case 'f':
		return r.spanFinish()
	default:
		return nil, r.r.ErrorHere(fmt.Errorf("unexpected object %q", tp))
	}
}

func (r *JSONReader) labels() (Labels, error) {
	if r.r.Type() != json.Array {
		return nil, r.r.ErrorHere(errors.New("array expected"))
	}

	var res Labels
	for r.r.HasNext() {
		l := string(r.r.NextString())
		res = append(res, l)
	}

	return res, nil
}

func (r *JSONReader) location() (Location, error) {
	if r.r.Type() != json.Object {
		return Location{}, r.r.ErrorHere(errors.New("object expected"))
	}

	var l Location

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
			return Location{}, r.r.ErrorHere(errors.New("unexpected key"))
		}
	}

	return l, nil
}

func (r *JSONReader) message() (Message, error) {
	if r.r.Type() != json.Object {
		return Message{}, r.r.ErrorHere(errors.New("object expected"))
	}

	var m Message
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
			s := r.r.NextString()
			_, err := hex.Decode(m.Span[:], s)
			if err != nil {
				return Message{}, r.r.ErrorHere(err)
			}
		default:
			return Message{}, r.r.ErrorHere(errors.New("unexpected key"))
		}
	}

	return m, nil
}

func (r *JSONReader) span() (Span, error) {
	if r.r.Type() != json.Object {
		return Span{}, r.r.ErrorHere(errors.New("object expected"))
	}

	var s Span
	for r.r.HasNext() {
		k := r.r.NextString()
		if len(k) == 0 {
			return Span{}, r.r.ErrorHere(errors.New("empty key"))
		}
		switch k[0] {
		case 'l':
			n := string(r.r.NextNumber())
			v, err := strconv.ParseUint(n, 10, 64)
			if err != nil {
				return Span{}, r.r.ErrorHere(err)
			}
			s.Location = uintptr(v)
		case 's':
			n := string(r.r.NextNumber())
			v, err := strconv.ParseInt(n, 10, 64)
			if err != nil {
				return Span{}, r.r.ErrorHere(err)
			}
			s.Started = time.Unix(0, v<<tlog.TimeReduction)
		case 'i':
			b := r.r.NextString()
			_, err := hex.Decode(s.ID[:], b)
			if err != nil {
				return Span{}, r.r.ErrorHere(err)
			}
		case 'p':
			b := r.r.NextString()
			_, err := hex.Decode(s.Parent[:], b)
			if err != nil {
				return Span{}, r.r.ErrorHere(err)
			}
		default:
			return Span{}, r.r.ErrorHere(errors.New("unexpected key"))
		}
	}

	return s, nil
}

func (r *JSONReader) spanFinish() (SpanFinish, error) {
	if r.r.Type() != json.Object {
		return SpanFinish{}, r.r.ErrorHere(errors.New("object expected"))
	}

	var s SpanFinish
	for r.r.HasNext() {
		k := r.r.NextString()
		if len(k) == 0 {
			return SpanFinish{}, r.r.ErrorHere(errors.New("empty key"))
		}
		switch k[0] {
		case 'i':
			b := r.r.NextString()
			_, err := hex.Decode(s.ID[:], b)
			if err != nil {
				return SpanFinish{}, r.r.ErrorHere(err)
			}
		case 'e':
			n := string(r.r.NextNumber())
			v, err := strconv.ParseInt(n, 10, 64)
			if err != nil {
				return SpanFinish{}, r.r.ErrorHere(err)
			}
			s.Elapsed = time.Duration(v << tlog.TimeReduction)
		}
	}

	return s, nil
}
