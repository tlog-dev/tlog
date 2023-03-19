package tlogcmd

import (
	"context"
	"errors"
	"io"
	"net"
	"time"
)

type (
	StoppableReader struct {
		Context context.Context
		Reader  ReadDeadliner
	}

	ReadDeadliner interface {
		io.Reader
		SetReadDeadline(time.Time) error
	}

	StoppableConn struct {
		Context context.Context
		net.Conn
	}

	StoppableListener struct {
		Context context.Context
		Listener
	}

	Listener interface {
		net.Listener
		SetDeadline(time.Time) error
	}
)

func NewStoppableReader(ctx context.Context, r io.Reader) io.Reader {
	rd, ok := r.(ReadDeadliner)
	if !ok {
		return r
	}

	return StoppableReader{
		Reader:  rd,
		Context: ctx,
	}
}

func (c StoppableReader) Read(p []byte) (n int, err error) {
	stopc := make(chan struct{})
	defer close(stopc)

	go func() {
		select {
		case <-c.Context.Done():
		case <-stopc:
			return
		}

		_ = c.Reader.SetReadDeadline(time.Unix(1, 0))
	}()

	n, err = c.Reader.Read(p)

	var nerr net.Error
	if errors.As(err, &nerr) && nerr.Timeout() {
		select {
		case <-c.Context.Done():
			err = c.Context.Err()
		default:
		}
	}

	return n, err
}

func (c StoppableConn) Read(p []byte) (n int, err error) {
	//	defer func() {
	//		tlog.Printw("stoppable read", "n", n, "err", err)
	//	}()

	stopc := make(chan struct{})
	defer close(stopc)

	go func() {
		select {
		case <-c.Context.Done():
		case <-stopc:
			return
		}

		_ = c.Conn.SetReadDeadline(time.Unix(1, 0))
	}()

	n, err = c.Conn.Read(p)

	var nerr net.Error
	if errors.As(err, &nerr) && nerr.Timeout() {
		select {
		case <-c.Context.Done():
			err = c.Context.Err()
		default:
		}
	}

	return n, err
}

func (c StoppableConn) ReadFrom(r io.Reader) (n int64, err error) {
	return readFrom(c.Conn, r)
}

func (c StoppableConn) WriteTo(w io.Writer) (n int64, err error) {
	return writeTo(w, c.Conn)
}

func (l StoppableListener) Accept() (c net.Conn, err error) {
	//	defer func() {
	//		tlog.Printw("stoppable accept", "n", n, "err", err)
	//	}()

	stopc := make(chan struct{})
	defer close(stopc)

	go func() {
		select {
		case <-l.Context.Done():
		case <-stopc:
			return
		}

		_ = l.Listener.SetDeadline(time.Unix(1, 0))
	}()

	c, err = l.Listener.Accept()

	var nerr net.Error
	if errors.As(err, &nerr) && nerr.Timeout() {
		select {
		case <-l.Context.Done():
			err = l.Context.Err()
		default:
		}
	}

	return c, err
}

func readFrom(w io.Writer, r io.Reader) (int64, error) {
	ww, ok := w.(io.ReaderFrom)
	if ok {
		return ww.ReadFrom(r)
	}

	return io.Copy(w, r)
}

func writeTo(w io.Writer, r io.Reader) (int64, error) {
	rr, ok := r.(io.WriterTo)
	if ok {
		return rr.WriteTo(w)
	}

	return io.Copy(w, r)
}
