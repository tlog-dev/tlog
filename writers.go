package tlog

import (
	"io"
	"sync/atomic"
	"testing"

	"github.com/nikandfor/errors"
	"github.com/nikandfor/tlog/wire"
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

	ReWriter struct {
		w io.Writer
		c io.Closer

		Open func(io.Writer, error) (io.Writer, error)

		d wire.Decoder

		ls, lsdebt []byte
	}

	// CountableIODiscard discards data but counts operations and bytes.
	// It's safe to use simultaneously (atimic operations are used).
	CountableIODiscard struct {
		Bytes, Operations int64
	}
)

func NewTeeWriter(ws ...io.Writer) (w TeeWriter) {
	return w.Append(ws...)
}

func (w TeeWriter) Append(ws ...io.Writer) TeeWriter {
	for _, s := range ws {
		if tee, ok := s.(TeeWriter); ok {
			w = append(w, tee...)
		} else {
			w = append(w, s)
		}
	}

	return w
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

func NewReWriter(open func(io.Writer, error) (io.Writer, error)) *ReWriter {
	return &ReWriter{
		Open: open,
	}
}

func (w *ReWriter) Write(p []byte) (n int, err error) {
	ls := w.detectHeaders(p)

	if w.w != nil {
		n, err = w.write(p)

		if err == nil {
			return
		}
	}

	n, err = w.open()
	if err != nil {
		return
	}

	if ls != nil {
		return
	}

	n, err = w.write(p)
	if err != nil {
		return
	}

	return
}

func (w *ReWriter) open() (n int, err error) {
	w.lsdebt = nil

	w.w, err = w.Open(w.w, err)
	if err != nil {
		return 0, errors.Wrap(err, "open")
	}

	w.c, _ = w.w.(io.Closer)

	if w.ls != nil {
		n, err = w.w.Write(w.ls)
		if err != nil {
			w.lsdebt = w.ls

			return
		}
	}

	return 0, nil
}

func (w *ReWriter) write(p []byte) (n int, err error) {
	if w.lsdebt != nil {
		_, err = w.w.Write(w.lsdebt)
		if err != nil {
			return
		}

		w.lsdebt = nil
	}

	return w.w.Write(p)
}

func (w *ReWriter) Close() error {
	if w.c == nil {
		return nil
	}

	return w.c.Close()
}

func (w *ReWriter) detectHeaders(p []byte) (ls []byte) {
	var e EventType

	var i int

	tag, els, i := w.d.Tag(p, i)
	if tag != wire.Map {
		return
	}

	var k []byte
	var sub int64

loop:
	for el := 0; els == -1 || el < int(els); el++ {
		if els == -1 && w.d.Break(p, &i) {
			break
		}

		k, i = w.d.String(p, i)

		tag, sub, _ = w.d.Tag(p, i)
		if tag != wire.Semantic {
			i = w.d.Skip(p, i)
			continue
		}

		switch {
		case sub == WireEventType && string(k) == KeyEventType:
			i = e.TlogParse(&w.d, p, i)

			break loop
		default:
			i = w.d.Skip(p, i)
		}
	}

	if e == EventLabels {
		w.ls = append(w.ls[:0], p...)
		w.lsdebt = nil
		ls = w.ls
	}

	return
}
