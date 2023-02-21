package tlio

import (
	"io"

	"github.com/nikandfor/errors"
)

type (
	WrappedCloser struct {
		io.Closer
		Msg  string
		Args []interface{}
	}

	CloserFunc func() error
)

func Close(f interface{}) error {
	c, ok := f.(io.Closer)
	if !ok {
		return nil
	}

	return c.Close()
}

func CloseWrap(f interface{}, name string, err *error) {
	c, ok := f.(io.Closer)
	if !ok {
		return
	}

	e := c.Close()
	if *err == nil {
		*err = errors.Wrap(e, "close %v", name)
	}
}

func WrapCloser(c io.Closer, msg string, args ...interface{}) io.Closer {
	return WrappedCloser{
		Closer: c,
		Msg:    msg,
		Args:   args,
	}
}

func WrapCloserFunc(cf func() error, msg string, args ...interface{}) io.Closer {
	return WrappedCloser{
		Closer: CloserFunc(cf),
		Msg:    msg,
		Args:   args,
	}
}

func (c WrappedCloser) Close() (err error) {
	err = c.Closer.Close()
	if err != nil {
		return errors.WrapDepth(err, 1, c.Msg, c.Args...)
	}

	return nil
}

func (c CloserFunc) Close() error { return c() }
