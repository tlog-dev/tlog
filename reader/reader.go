// +build ignore

package tlog

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/nikandfor/json"
)

type (
	Reader interface {
		Read() interface{}
	}

	LocationInfo struct {
		PC   Location
		Func string
		File string
		Line int
	}

	SpanFinish struct {
		ID      ID
		Elapsed time.Duration
		Flags   int
	}

	JSONReader struct {
		r *json.Reader
	}
)

func NewJSONReader(r *json.Reader) *JSONReader {
	return &JSONReader{r: r}
}

func (r *JSONReader) Read() interface{} {
	if err := r.r.Err(); err != nil {
		return err
	}
	if !r.r.HasNext() {
		return io.EOF
	}
	defer r.r.HasNext()

	tp := r.r.NextString()
	if len(tp) != 1 {
		return r.r.ErrorHere(fmt.Errorf("unexpected object %q", tp))
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
		return r.r.ErrorHere(fmt.Errorf("unexpected object %q", tp))
	}
}

func (r *JSONReader) labels() interface{} {
	if r.r.Type() != json.Array {
		return r.r.ErrorHere(errors.New("array expected"))
	}

	var res Labels
	for r.r.HasNext() {
		l := string(r.r.NextString())
		res = append(res, l)
	}

	return res
}

func (r *JSONReader) location() interface{} {
	if r.r.Type() != json.Object {
		return r.r.ErrorHere(errors.New("object expected"))
	}

	var l LocationInfo

	for r.r.HasNext() {
		k := r.r.NextString()
		if len(k) == 0 {
			return r.r.ErrorHere(errors.New("empty key"))
		}
		switch k[0] {
		case 'f':
			l.File = string(r.r.NextString())
		case 'n':
			l.Func = string(r.r.NextString())
		case 'p':
			n := string(r.r.NextNumber())
			v, err := strconv.ParseUint(n, 10, 64)
			if err != nil {
				return r.r.ErrorHere(err)
			}
			l.PC = Location(v)
		case 'l':
			n := string(r.r.NextNumber())
			v, err := strconv.ParseUint(n, 10, 64)
			if err != nil {
				return r.r.ErrorHere(err)
			}
			l.Line = int(v)
		default:
			return r.r.ErrorHere(errors.New("unexpected key"))
		}
	}

	return &l
}

func (r *JSONReader) message() interface{} {
	if r.r.Type() != json.Object {
		return r.r.ErrorHere(errors.New("object expected"))
	}

	var m Message
	for r.r.HasNext() {
		k := r.r.NextString()
		if len(k) == 0 {
			return r.r.ErrorHere(errors.New("empty key"))
		}
		switch k[0] {
		case 'm':
			m.Format = string(r.r.NextString())
		case 'l':
			n := string(r.r.NextNumber())
			v, err := strconv.ParseUint(n, 10, 64)
			if err != nil {
				return r.r.ErrorHere(err)
			}
			m.Location = Location(v)
		case 't':
			n := string(r.r.NextNumber())
			v, err := strconv.ParseInt(n, 10, 64)
			if err != nil {
				return r.r.ErrorHere(err)
			}
			m.Time = time.Duration(v * 1000)
		case 's':
			n := string(r.r.NextNumber())
			v, err := strconv.ParseInt(n, 10, 64)
			if err != nil {
				return r.r.ErrorHere(err)
			}
			m.Args = []interface{}{ID(v)}
		default:
			return r.r.ErrorHere(errors.New("unexpected key"))
		}
	}

	return &m
}

func (r *JSONReader) span() interface{} {
	if r.r.Type() != json.Object {
		return r.r.ErrorHere(errors.New("object expected"))
	}

	var s Span
	for r.r.HasNext() {
		k := r.r.NextString()
		if len(k) == 0 {
			return r.r.ErrorHere(errors.New("empty key"))
		}
		switch k[0] {
		case 'l':
			n := string(r.r.NextNumber())
			v, err := strconv.ParseUint(n, 10, 64)
			if err != nil {
				return r.r.ErrorHere(err)
			}
			s.Location = Location(v)
		case 's':
			n := string(r.r.NextNumber())
			v, err := strconv.ParseInt(n, 10, 64)
			if err != nil {
				return r.r.ErrorHere(err)
			}
			s.Started = time.Unix(0, v*1000)
		case 'i':
			n := string(r.r.NextNumber())
			v, err := strconv.ParseInt(n, 10, 64)
			if err != nil {
				return r.r.ErrorHere(err)
			}
			s.ID = ID(v)
		case 'p':
			n := string(r.r.NextNumber())
			v, err := strconv.ParseInt(n, 10, 64)
			if err != nil {
				return r.r.ErrorHere(err)
			}
			s.Parent = ID(v)
		default:
			return r.r.ErrorHere(errors.New("unexpected key"))
		}
	}

	return &s
}

func (r *JSONReader) spanFinish() interface{} {
	if r.r.Type() != json.Object {
		return r.r.ErrorHere(errors.New("object expected"))
	}

	var s SpanFinish
	for r.r.HasNext() {
		k := r.r.NextString()
		if len(k) == 0 {
			return r.r.ErrorHere(errors.New("empty key"))
		}
		switch k[0] {
		case 'i':
			n := string(r.r.NextNumber())
			v, err := strconv.ParseInt(n, 10, 64)
			if err != nil {
				return r.r.ErrorHere(err)
			}
			s.ID = ID(v)
		case 'e':
			n := string(r.r.NextNumber())
			v, err := strconv.ParseInt(n, 10, 64)
			if err != nil {
				return r.r.ErrorHere(err)
			}
			s.Elapsed = time.Duration(v * 1000)
		case 'F':
			n := string(r.r.NextNumber())
			v, err := strconv.ParseInt(n, 10, 64)
			if err != nil {
				return r.r.ErrorHere(err)
			}
			s.Flags = int(v)
		}
	}

	return &s
}
