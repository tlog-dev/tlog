package tlio

import (
	"io"

	"github.com/nikandfor/errors"
)

type (
	WrapperCloser struct {
		io.Closer
		Msg  string
		Args []interface{}
	}

	CloserFunc func() error
)

func WrapCloser(c io.Closer, msg string, args ...interface{}) io.Closer {
	return WrapperCloser{
		Closer: c,
		Msg:    msg,
		Args:   args,
	}
}

func WrapCloserFunc(cf func() error, msg string, args ...interface{}) io.Closer {
	return WrapperCloser{
		Closer: CloserFunc(cf),
		Msg:    msg,
		Args:   args,
	}
}

func (c WrapperCloser) Close() (err error) {
	err = c.Closer.Close()
	if err != nil {
		return errors.WrapDepth(err, 1, c.Msg, c.Args...)
	}

	return nil
}

func (c CloserFunc) Close() error { return c() }
