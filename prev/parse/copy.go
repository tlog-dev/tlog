package parse

import (
	"github.com/nikandfor/errors"
)

//nolint:gocognit
func Copy(w Writer, r LowReader) error {
	for {
		tp, err := r.Type()
		if err != nil {
			return errors.Wrap(err, "reader")
		}

		switch rune(tp) {
		case 'L':
			ls, err := r.Labels()
			if err != nil {
				return errors.Wrap(err, "reader")
			}

			err = w.Labels(ls)
		case 'l':
			l, err := r.Frame()
			if err != nil {
				return errors.Wrap(err, "reader")
			}

			err = w.Frame(l)
		case 'M':
			m, err := r.Meta()
			if err != nil {
				return errors.Wrap(err, "reader")
			}

			err = w.Meta(m)
		case 'm':
			m, err := r.Message()
			if err != nil {
				return errors.Wrap(err, "reader")
			}

			err = w.Message(m)
		case 'v':
			m, err := r.Metric()
			if err != nil {
				return errors.Wrap(err, "reader")
			}

			err = w.Metric(m)
		case 's':
			s, err := r.SpanStart()
			if err != nil {
				return errors.Wrap(err, "reader")
			}

			err = w.SpanStart(s)
		case 'f':
			f, err := r.SpanFinish()
			if err != nil {
				return errors.Wrap(err, "reader")
			}

			err = w.SpanFinish(f)
		default:
			_, err = r.Read()
			if err != nil {
				return errors.Wrap(err, "reader")
			}

			return errors.New("unsupported record: %v", tp)
		}

		if err != nil {
			return errors.Wrap(err, "writer")
		}
	}
}
