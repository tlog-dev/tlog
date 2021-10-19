package tlio

import (
	"bytes"
	"io"
	"sync/atomic"
	"testing"

	"github.com/nikandfor/errors"
	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/wire"
)

type (
	TeeWriter []io.Writer

	NopCloser struct {
		io.Reader
		io.Writer
	}

	WriteCloser struct {
		io.Writer
		io.Closer
	}

	ReWriter struct {
		w io.Writer
		c io.Closer

		Open func(io.Writer, error) (io.Writer, error)
	}

	DeLabels struct {
		w io.Writer
		d wire.Decoder
		e wire.Encoder

		b, ls []byte
	}

	TailWriter struct {
		w io.Writer
		n int

		i   int
		buf [][]byte
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
	return Fd(c.Writer)
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
	if w.w != nil {
		n, err = w.w.Write(p)

		if err == nil {
			return
		}
	}

	n, err = w.open()
	if err != nil {
		return
	}

	n, err = w.w.Write(p)
	if err != nil {
		return
	}

	return
}

func (w *ReWriter) open() (n int, err error) {
	w.w, err = w.Open(w.w, err)
	if err != nil {
		return 0, errors.Wrap(err, "open")
	}

	w.c, _ = w.w.(io.Closer)

	return 0, nil
}

func (w *ReWriter) Close() error {
	if w.c == nil {
		return nil
	}

	return w.c.Close()
}

func NewDeLabels(w io.Writer) *DeLabels {
	return &DeLabels{
		w: w,
	}
}

func (w *DeLabels) Write(p []byte) (i int, err error) {
	tag, els, i := w.d.Tag(p, i)
	if tag != wire.Map {
		return i, errors.New("map expected")
	}

	gst := i

	var k []byte
	var st int
	var sub int64
	for el := 0; els == -1 || el < int(els); el++ {
		if els == -1 && w.d.Break(p, &i) {
			break
		}

		st = i

		k, i = w.d.String(p, i)
		tag, sub, i = w.d.SkipTag(p, i)

		if sub == tlog.WireLabels && string(k) == tlog.KeyLabels {
			break
		}
	}

	if !bytes.Equal(w.ls, p[st:i]) {
		w.ls = append(w.ls[:0], p[st:i]...)

		return w.w.Write(p)
	}

	w.b = w.b[:0]

	if els != -1 {
		w.b = w.e.AppendMap(w.b, int(els-1))
	} else {
		gst = 0
	}

	w.b = append(w.b, p[gst:st]...)
	w.b = append(w.b, p[i:]...)

	i, err = w.w.Write(w.b)
	if err != nil {
		return i, err
	}

	return len(p), nil
}

func NewTailWriter(w io.Writer, n int) *TailWriter {
	return &TailWriter{
		w:   w,
		n:   n,
		buf: make([][]byte, n),
	}
}

func (w *TailWriter) Write(p []byte) (n int, err error) {
	i := w.i % w.n
	w.buf[i] = append(w.buf[i][:0], p...)

	w.i++

	return len(p), nil
}

func (w *TailWriter) Flush() (err error) {
	for i := w.i; i < w.i+w.n; i++ {
		b := w.buf[i%w.n]

		if len(b) == 0 {
			continue
		}

		_, err = w.w.Write(b)
		if err != nil {
			return err
		}

		w.buf[i%w.n] = b[:0]
	}

	return nil
}

func Fd(f interface{}) uintptr {
	const ffff = ^uintptr(0)

	if f == nil {
		return ffff
	}

	switch f := f.(type) {
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
