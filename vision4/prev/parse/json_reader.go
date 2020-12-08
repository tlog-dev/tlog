//nolint:goerr113
package parse

import (
	"encoding/hex"
	"fmt"
	"io"
	"strconv"

	"github.com/nikandfor/errors"
	"github.com/nikandfor/json"

	"github.com/nikandfor/tlog"
)

type JSONReader struct {
	r      *json.Reader
	err    error
	tp     Type
	finish bool

	SkipUnknown bool

	l *tlog.Logger
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

	if r.tp != 0 {
		return r.tp, nil
	}

	if r.finish {
		if r.r.HasNext() {
			return 0, r.wraperr(fmt.Errorf("expected end of object, got %v", r.r.Type()))
		}
	}

next:
	if !r.r.HasNext() {
		if err := r.r.Err(); err != nil {
			return 0, r.wraperr(err)
		}

		return 0, r.wraperr(io.EOF)
	}
	r.finish = true

	tp := r.r.NextString()
	if len(tp) != 1 {
		return 0, r.wraperr(fmt.Errorf("unexpected object %q", tp))
	}

	if r.l.V("tag") != nil {
		r.l.Printf("record tag: %v type %v", Type(tp[0]), r.r.Type())
	}

	switch tp[0] {
	case 'L', 'l', 'm', 'v', 'M', 's', 'f':
		r.tp = Type(tp[0])
		return r.tp, nil
	default:
		if err := r.unknownField(tp); err != nil {
			r.tp = 0
			return 0, err
		}

		goto next
	}
}

func (r *JSONReader) Any() (interface{}, error) {
	if r.err != nil {
		return 0, r.err
	}

	switch rune(r.tp) {
	case 'L':
		return r.Labels()
	case 'l':
		return r.Frame()
	case 'm':
		return r.Message()
	case 'v':
		return r.Metric()
	case 'M':
		return r.Meta()
	case 's':
		return r.SpanStart()
	case 'f':
		return r.SpanFinish()
	default:
		return nil, r.r.ErrorHere(fmt.Errorf("unexpected record %q", r.tp))
	}
}

func (r *JSONReader) Read() (interface{}, error) {
	_, _ = r.Type()
	return r.Any()
}

func (r *JSONReader) Labels() (ls Labels, err error) {
	if r.r.Type() != json.Object {
		return Labels{}, r.r.ErrorHere(fmt.Errorf("object expected, got %v %v", r.r.Type(), r.tp))
	}

	for r.r.HasNext() {
		k := r.r.NextString()
		if len(k) == 0 {
			return Labels{}, r.r.ErrorHere(errors.New("empty key"))
		}
		switch k[0] {
		case 's':
			ls.Span, err = r.id()
			if err != nil {
				return Labels{}, r.r.ErrorHere(err)
			}
		case 'L':
			if r.r.Type() != json.Array {
				return Labels{}, r.r.ErrorHere(fmt.Errorf("array expected, got %v %v", r.r.Type(), r.tp))
			}

			for r.r.HasNext() {
				l := string(r.r.NextString())
				ls.Labels = append(ls.Labels, l)
			}
		default:
			if err := r.unknownField(k); err != nil {
				return Labels{}, err
			}
		}
	}

	if r.l.V("record") != nil {
		r.l.Printf("labels: %v spanid %v", ls.Labels, ls.Span)
	}

	r.tp = 0

	return ls, nil
}

func (r *JSONReader) Frame() (l Frame, err error) {
	if r.r.Type() != json.Object {
		return Frame{}, r.r.ErrorHere(errors.New("object expected"))
	}

	for r.r.HasNext() {
		k := r.r.NextString()
		if len(k) == 0 {
			return Frame{}, r.r.ErrorHere(errors.New("empty key"))
		}
		switch k[0] {
		case 'f':
			l.File = string(r.r.NextString())
		case 'n':
			l.Name = string(r.r.NextString())
		case 'p':
			n := string(r.r.NextNumber())
			l.PC, err = strconv.ParseUint(n, 10, 64)
			if err != nil {
				return Frame{}, r.r.ErrorHere(err)
			}
		case 'e':
			n := string(r.r.NextNumber())
			l.Entry, err = strconv.ParseUint(n, 10, 64)
			if err != nil {
				return Frame{}, r.r.ErrorHere(err)
			}
		case 'l':
			n := string(r.r.NextNumber())
			v, err := strconv.ParseUint(n, 10, 64)
			if err != nil {
				return Frame{}, r.r.ErrorHere(err)
			}
			l.Line = int(v)
		default:
			if err := r.unknownField(k); err != nil {
				return Frame{}, err
			}
		}
	}

	if r.l.V("record") != nil {
		r.l.Printf("location: %v", l)
	}

	r.tp = 0

	return l, nil
}

func (r *JSONReader) Message() (m Message, err error) {
	if r.r.Type() != json.Object {
		return Message{}, r.r.ErrorHere(errors.New("object expected"))
	}

	for r.r.HasNext() {
		k := r.r.NextString()
		if len(k) == 0 {
			return m, r.r.ErrorHere(errors.New("empty key"))
		}

		switch k[0] {
		case 'm':
			m.Text = string(r.r.NextString())
		case 'l':
			n := string(r.r.NextNumber())
			m.PC, err = strconv.ParseUint(n, 10, 64)
		case 't':
			n := string(r.r.NextNumber())
			m.Time, err = strconv.ParseInt(n, 10, 64)
		case 's':
			m.Span, err = r.id()
		case 'i':
			n := r.r.NextNumber()
			if len(n) == 0 {
				err = errors.New("bad level")
			}

			v, err := strconv.ParseInt(string(n), 10, 32)
			if err != nil {
				break
			}

			m.Level = Level(v)
		case 'a':
			m.Attrs, err = r.messageAttrs()
		default:
			err = r.unknownField(k)
		}

		if err != nil {
			return m, r.r.ErrorHere(err)
		}
	}

	if r.l.V("record") != nil {
		r.l.Printf("message: %v", m)
	}

	r.tp = 0

	return m, nil
}

func (r *JSONReader) messageAttrs() (as Attrs, err error) {
	if r.r.Type() != json.Array {
		return as, r.r.ErrorHere(errors.New("array expected"))
	}

	for r.r.HasNext() {
		if r.r.Type() != json.Object {
			return as, r.r.ErrorHere(errors.New("object expected"))
		}

		var a Attr

		var tp byte
		val, wr := tlog.Getbuf()
		defer wr.Ret(&val)

		for r.r.HasNext() {
			k := r.r.NextString()
			if len(k) == 0 {
				return as, r.r.ErrorHere(errors.New("empty key"))
			}

			switch k[0] {
			case 'n':
				a.Name = string(r.r.NextString())
			case 't':
				v := r.r.NextString()
				if len(v) == 0 {
					return as, r.r.ErrorHere(errors.New("empty attr type"))
				}

				tp = v[0]
			case 'v':
				v := r.r.NextAsBytes()
				val = append(val[:0], v...)
			default:
				if err = r.unknownField(k); err != nil {
					return
				}
			}
		}

		switch tp {
		case 'd':
			v := json.Wrap(val).NextString()
			a.Value, err = tlog.IDFromStringAsBytes(v)
		case 'i':
			a.Value, err = strconv.ParseInt(string(val), 10, 64)
		case 'u':
			a.Value, err = strconv.ParseUint(string(val), 10, 64)
		case 'f':
			a.Value, err = strconv.ParseFloat(string(val), 64)
		case 's':
			v := json.Wrap(val).NextString()
			a.Value = string(v)
		case '?':
			v := json.Wrap(val).NextString()
			a.Value = errors.New(string(v))
		default:
			a.Value = errors.New("unsupported field type: '%c'", tp)
		}

		as = append(as, a)
	}

	return
}

func (r *JSONReader) Metric() (m Metric, err error) {
	if r.r.Type() != json.Object {
		return Metric{}, r.r.ErrorHere(errors.New("object expected"))
	}

	for r.r.HasNext() {
		k := r.r.NextString()
		if len(k) == 0 {
			return Metric{}, r.r.ErrorHere(errors.New("empty key"))
		}
		switch k[0] {
		case 'n':
			m.Name = string(r.r.NextString())
		case 'v':
			n := string(r.r.NextNumber())
			m.Value, err = strconv.ParseFloat(n, 64)
			if err != nil {
				return Metric{}, r.r.ErrorHere(err)
			}
		case 's':
			m.Span, err = r.id()
			if err != nil {
				return Metric{}, r.r.ErrorHere(err)
			}
		case 'L':
			if r.r.Type() != json.Array {
				return Metric{}, r.r.ErrorHere(fmt.Errorf("array expected, got %v %v", r.r.Type(), r.tp))
			}

			for r.r.HasNext() {
				l := string(r.r.NextString())
				m.Labels = append(m.Labels, l)
			}
		default:
			if err := r.unknownField(k); err != nil {
				return Metric{}, err
			}
		}
	}

	if r.l.V("record") != nil {
		r.l.Printf("metric: %v", m)
	}

	r.tp = 0

	return m, nil
}

func (r *JSONReader) Meta() (m Meta, err error) {
	if r.r.Type() != json.Object {
		return Meta{}, r.r.ErrorHere(errors.New("object expected"))
	}

	for r.r.HasNext() {
		k := r.r.NextString()
		if len(k) == 0 {
			return Meta{}, r.r.ErrorHere(errors.New("empty key"))
		}
		switch k[0] {
		case 't':
			m.Type = string(r.r.NextString())
		case 'd':
			if r.r.Type() != json.Array {
				return Meta{}, r.r.ErrorHere(fmt.Errorf("array expected, got %v %v", r.r.Type(), r.tp))
			}

			for r.r.HasNext() {
				l := string(r.r.NextString())
				m.Data = append(m.Data, l)
			}
		default:
			if err := r.unknownField(k); err != nil {
				return Meta{}, err
			}
		}
	}

	if r.l.V("record") != nil {
		r.l.Printf("meta: %v", m)
	}

	r.tp = 0

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
			s.PC, err = strconv.ParseUint(n, 10, 64)
			if err != nil {
				return SpanStart{}, r.r.ErrorHere(err)
			}
		case 's':
			n := string(r.r.NextNumber())
			s.StartedAt, err = strconv.ParseInt(n, 10, 64)
			if err != nil {
				return SpanStart{}, r.r.ErrorHere(err)
			}
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
			if err := r.unknownField(k); err != nil {
				return SpanStart{}, err
			}
		}
	}

	if r.l.V("record") != nil {
		r.l.Printf("span start: %v", s)
	}

	r.tp = 0

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
			f.Elapsed, err = strconv.ParseInt(n, 10, 64)
			if err != nil {
				return SpanFinish{}, r.r.ErrorHere(err)
			}
		default:
			if err := r.unknownField(k); err != nil {
				return SpanFinish{}, err
			}
		}
	}

	if r.l.V("record") != nil {
		r.l.Printf("span finish: %v", f)
	}

	r.tp = 0

	return f, nil
}

func (r *JSONReader) unknownField(k []byte) error {
	if r.SkipUnknown {
		if r.l.If("skip") {
			r.l.PrintfDepth(1, "skip key %q", k)
		}

		r.r.Skip()
	}

	return r.wraperr(fmt.Errorf("unexpected field %q", k))
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
	if errors.Is(err, io.EOF) {
		r.err = errors.Unwrap(err)
		return err
	}
	r.err = r.r.ErrorHere(err)
	return r.err
}
