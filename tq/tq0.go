//go:build ignore

package tq

import (
	"errors"
	"io"

	"github.com/nikandfor/tlog/wire"
)

type (
	Base struct {
		io.Writer
		io.Reader

		wire.LowDecoder

		b []byte
	}

	Keys struct {
		Base
	}

	Array struct {
		Base
	}

	Reader interface {
		NextReader() (io.Reader, error)
	}

	Writer interface {
		NextWriter() (io.Writer, error)
	}
)

func (f *Keys) Process(r Reader, w Writer) (err error) {
}

func (f *Keys) Read(p []byte) (n int, err error) {
}

func (f *Keys) Write(p []byte) (i int, err error) {
	tag, sub, i := f.Tag(p, 0)
	if tag != wire.Map {
		return 0, errors.New("map expected")
	}

	for el := 0; sub == -1 || el < int(sub); el++ {
		if sub == -1 && f.Break(p, i) {
			break
		}

		st := i
		i = f.Skip(p, i)

		_, err = f.Writer.Write(p[st:i])
		if err != nil {
			return st, errors.Wrap(err, "%s", f.Writer)
		}

		i = f.Skip(p, i)
	}

	return i, nil
}

func (f Array) Write(p []byte) (i int, err error) {
	f.b = append(f.b, Array|LenBreak)

	f.b = append(f.b, p...)

	f.b = append(f.b, Special|Break)

	_, err = f.Writer.Write(f.b)

	return len(p), err
}
