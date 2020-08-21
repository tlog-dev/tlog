package parse

import (
	"io"

	"github.com/nikandfor/errors"
)

func Copy(w Writer, r LowReader) error {
	for {
		tp, err := r.Type()
		if err == io.EOF {
			return nil
		}
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
			if err != nil {
				return errors.Wrap(err, "writer")
			}
		case 'l':
			l, err := r.Location()
			if err != nil {
				return errors.Wrap(err, "reader")
			}

			err = w.Location(l)
			if err != nil {
				return errors.Wrap(err, "writer")
			}
		case 'm':
			m, err := r.Message()
			if err != nil {
				return errors.Wrap(err, "reader")
			}

			err = w.Message(m)
			if err != nil {
				return errors.Wrap(err, "writer")
			}
		case 'v':
			m, err := r.Metric()
			if err != nil {
				return errors.Wrap(err, "reader")
			}

			err = w.Metric(m)
			if err != nil {
				return errors.Wrap(err, "writer")
			}
		case 's':
			s, err := r.SpanStart()
			if err != nil {
				return errors.Wrap(err, "reader")
			}

			err = w.SpanStart(s)
			if err != nil {
				return errors.Wrap(err, "writer")
			}
		case 'f':
			f, err := r.SpanFinish()
			if err != nil {
				return errors.Wrap(err, "reader")
			}

			err = w.SpanFinish(f)
			if err != nil {
				return errors.Wrap(err, "writer")
			}
		default:
			_, err = r.Read()
			if err != nil {
				return errors.Wrap(err, "reader")
			}
		}
	}

	return nil
}
