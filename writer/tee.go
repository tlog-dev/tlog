package writer

import "io"

type (
	Tee []io.Writer
)

func NewTee(ws ...io.Writer) Tee {
	return Tee(ws)
}

func (w Tee) Append(ws ...io.Writer) Tee {
	return append(w, ws...)
}

func (w Tee) Write(p []byte) (n int, err error) {
	for i, w := range w {
		m, e := w.Write(p)

		if i == 0 {
			n = m
		}

		if err == nil {
			err = e
		}
	}

	return
}

func (w Tee) Close() (err error) {
	for _, w := range w {
		c, ok := w.(io.Closer)
		if !ok {
			continue
		}

		e := c.Close()

		if err == nil {
			err = e
		}
	}

	return
}
