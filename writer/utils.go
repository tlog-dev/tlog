package writer

import (
	"io"
	"sync/atomic"
	"testing"
)

type (
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
		B, N int64
	}
)

func (NopCloser) Close() error { return nil }

func (w *CountableIODiscard) ReportDisk(b *testing.B) {
	b.ReportMetric(float64(w.B)/float64(b.N), "disk_B/op")
}

func (w *CountableIODiscard) Write(p []byte) (int, error) {
	atomic.AddInt64(&w.N, 1)
	atomic.AddInt64(&w.B, int64(len(p)))

	return len(p), nil
}
