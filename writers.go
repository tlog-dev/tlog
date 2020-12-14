package tlog

import (
	"io"
	"sync/atomic"
	"testing"
)

type (
	TeeWriter []io.Writer

	NopCloser struct {
		io.Writer
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

func (w *CountableIODiscard) ReportDisk(b *testing.B) {
	b.ReportMetric(float64(w.Bytes)/float64(b.N), "disk_B/op")
}

func (w *CountableIODiscard) Write(p []byte) (int, error) {
	atomic.AddInt64(&w.Operations, 1)
	atomic.AddInt64(&w.Bytes, int64(len(p)))

	return len(p), nil
}
