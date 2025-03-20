package tlio

import (
	"io"

	"tlog.app/go/errors"
)

type (
	WrappedCloser struct {
		io.Closer
		Msg  string
		Args []interface{}
	}

	CloserFunc func() error

	MultiCloser []io.Closer
)

func Close(f interface{}) error {
	c, ok := f.(io.Closer)
	if !ok {
		return nil
	}

	return c.Close()
}

func CloseWrap(f interface{}, name string, errp *error) { //nolint:gocritic
	e := Close(f)
	if *errp == nil && e != nil {
		*errp = errors.Wrap(e, "close %v", name)
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
		return errors.Wrap(err, c.Msg, c.Args...)
	}

	return nil
}

func (c CloserFunc) Close() error { return c() }

func (c MultiCloser) Close() (err error) {
	for i, c := range c {
		e := c.Close()
		if err == nil && e != nil {
			err = errors.Wrap(e, "multi #%d (%T)", i, c)
		}
	}

	return err
}
