package tlog

import (
	"io"
	"sync/atomic"
	"testing"
)

type (
	TeeWriter []io.Writer

	NopCloser struct {
		io.Reader
		io.Writer
	}

	ReadCloser struct {
		io.Reader
		io.Closer
	}

	WriteCloser struct {
		io.Writer
		io.Closer
	}

	// CountableIODiscard discards data but counts operations and bytes.
	// It's safe to use simultaneously (atimic operations are used).
	CountableIODiscard struct {
		Bytes, Operations int64
	}
)

func NewTeeWriter(ws ...io.Writer) TeeWriter {
	if len(ws) == 0 {
		return TeeWriter{}
	}

	if t, ok := ws[0].(TeeWriter); ok {
		return append(t, ws[1:]...)
	}

	return TeeWriter(ws)
}

func (w TeeWriter) Append(ws ...io.Writer) TeeWriter {
	return append(w, ws...)
}

func (w TeeWriter) Write(p []byte) (n int, err error) {
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

func (w TeeWriter) Close() (err error) {
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

func (NopCloser) Close() error { return nil }

func (c NopCloser) Fd() uintptr {
	const ffff = ^uintptr(0)

	if c.Writer == nil {
		return ffff
	}

	switch f := c.Writer.(type) {
	case interface {
		Fd() uintptr
	}:
		return f.Fd()
	case interface {
		Fd() int
	}:
		return uintptr(f.Fd())
	}

	return ffff
}

func (w *CountableIODiscard) ReportDisk(b *testing.B) {
	b.ReportMetric(float64(w.Bytes)/float64(b.N), "disk_B/op")
}

func (w *CountableIODiscard) Write(p []byte) (int, error) {
	atomic.AddInt64(&w.Operations, 1)
	atomic.AddInt64(&w.Bytes, int64(len(p)))

	return len(p), nil
}
