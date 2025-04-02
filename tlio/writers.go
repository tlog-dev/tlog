package tlio

import (
	"bytes"
	"io"
	"sync/atomic"
	"testing"

	"tlog.app/go/errors"

	"tlog.app/go/tlog"
	"tlog.app/go/tlog/tlwire"
)

type (
	MultiWriter []io.Writer

	ReWriter struct {
		io.Writer
		io.Closer

		Open func(io.Writer, error) (io.Writer, error)
	}

	// DeLabels removes repeating Labels from events.
	DeLabels struct {
		io.Writer

		d tlwire.Decoder
		e tlwire.Encoder

		b, ls []byte
	}

	TailWriter struct {
		io.Writer
		RingWriter
	}

	HeadWriter struct {
		io.Writer
		N int
	}

	RingWriter struct {
		n int

		i   int
		buf [][]byte
	}

	// CountingIODiscard discards data but counts writes and bytes.
	// It's safe to use simultaneously (atomic operations are used).
	CountingIODiscard struct {
		Bytes, Writes atomic.Int64
	}

	SandwichWriter struct {
		io.Writer
		sandwichFilling
	}

	sandwichFilling struct {
		io.Writer
		n int
	}

	WriterFunc func(p []byte) (int, error)

	// Base interfaces.

	Flusher interface {
		Flush() error
	}

	FlusherNoError interface {
		Flush()
	}

	NopCloser struct {
		io.Reader
		io.Writer
	}

	WriteCloser struct {
		io.Writer
		io.Closer
	}

	WriteFlusher struct {
		io.Writer
		Flusher
	}

	wrapFlusher struct {
		FlusherNoError
	}
)

func NewMultiWriter(ws ...io.Writer) (w MultiWriter) {
	return w.Append(ws...)
}

func (w MultiWriter) Append(ws ...io.Writer) MultiWriter {
	w = make(MultiWriter, 0, len(ws))

	for _, s := range ws {
		if s == nil {
			continue
		}

		if tee, ok := s.(MultiWriter); ok {
			w = append(w, tee...)
		} else {
			w = append(w, s)
		}
	}

	return w
}

func (w MultiWriter) Write(p []byte) (n int, err error) {
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

func (w MultiWriter) Close() (err error) {
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

func (w MultiWriter) WalkWriter(f func(w io.Writer) error) error {
	for _, w := range w {
		err := f(w)
		if err != nil {
			return err
		}

		err = WalkWriter(w, f)
		if err != nil {
			return errors.Wrap(err, "%T", w)
		}
	}

	return nil
}

func (NopCloser) Close() error { return nil }

func (c NopCloser) Fd() uintptr {
	return Fd(c.Writer)
}

func (w *CountingIODiscard) ReportDisk(b *testing.B) {
	b.ReportMetric(float64(w.Bytes.Load())/float64(b.N), "disk_B/op")
}

func (w *CountingIODiscard) Write(p []byte) (int, error) {
	w.Writes.Add(1)
	w.Bytes.Add(int64(len(p)))

	return len(p), nil
}

func NewReWriter(open func(io.Writer, error) (io.Writer, error)) *ReWriter {
	return &ReWriter{
		Open: open,
	}
}

func (w *ReWriter) Write(p []byte) (n int, err error) {
	if w.Writer != nil {
		n, err = w.Writer.Write(p)
		if err == nil {
			return n, nil
		}
	}

	err = w.open()
	if err != nil {
		return
	}

	n, err = w.Writer.Write(p)
	if err != nil {
		return
	}

	return
}

func (w *ReWriter) open() (err error) {
	w.Writer, err = w.Open(w.Writer, err)
	if err != nil {
		return errors.Wrap(err, "open")
	}

	w.Closer, _ = w.Writer.(io.Closer)

	return nil
}

func (w *ReWriter) Close() error {
	if w.Closer == nil {
		return nil
	}

	return w.Closer.Close()
}

func NewDeLabels(w io.Writer) *DeLabels {
	return &DeLabels{
		Writer: w,
	}
}

func (w *DeLabels) Write(p []byte) (i int, err error) {
	tag, els, i := w.d.Tag(p, i)
	if tag != tlwire.Map {
		return i, errors.New("map expected")
	}

	gst := i

	var st int
	var sub int64
	for el := 0; els == -1 || el < int(els); el++ {
		if els == -1 && w.d.Break(p, &i) {
			break
		}

		st = i

		_, i = w.d.Bytes(p, i)
		_, sub, i = w.d.SkipTag(p, i)

		if sub == tlog.WireLabel {
			break
		}
	}

	if !bytes.Equal(w.ls, p[st:i]) {
		w.ls = append(w.ls[:0], p[st:i]...)

		return w.Writer.Write(p)
	}

	w.b = w.b[:0]

	if els != -1 {
		w.b = w.e.AppendMap(w.b, int(els-1))
	} else {
		gst = 0
	}

	w.b = append(w.b, p[gst:st]...)
	w.b = append(w.b, p[i:]...)

	i, err = w.Writer.Write(w.b)
	if err != nil {
		return i, err
	}

	return len(p), nil
}

func (w *DeLabels) Unwrap() interface{} {
	return w.Writer
}

func NewTailWriter(w io.Writer, n int) *TailWriter {
	return &TailWriter{
		Writer:     w,
		RingWriter: RingWriter{},
	}
}

func (w *TailWriter) Flush() (err error) {
	_, err = w.RingWriter.WriteTo(w.Writer)
	if err != nil {
		return err
	}

	w.RingWriter.Reset()

	if f, ok := w.Writer.(Flusher); ok {
		return f.Flush()
	}

	return nil
}

func (w *TailWriter) Unwrap() interface{} {
	return w.Writer
}

func NewRingWriter(n int) *RingWriter {
	return &RingWriter{
		n:   n,
		buf: make([][]byte, n),
	}
}

func (w *RingWriter) Write(p []byte) (n int, err error) {
	i := w.i % w.n
	w.buf[i] = append(w.buf[i][:0], p...)

	w.i++

	return len(p), nil
}

func (w *RingWriter) WriteTo(wr io.Writer) (n int64, err error) {
	for i := w.i; i < w.i+w.n; i++ {
		b := w.buf[i%w.n]

		if len(b) == 0 {
			continue
		}

		m, err := wr.Write(b)
		n += int64(m)
		if err != nil {
			return n, err
		}
	}

	return n, nil
}

func (w *RingWriter) Reset() {
	for i := range w.buf {
		w.buf[i] = w.buf[i][:0]
	}
}

func NewHeadWriter(w io.Writer, n int) *HeadWriter {
	return &HeadWriter{
		Writer: w,
		N:      n,
	}
}

func (w *HeadWriter) Write(p []byte) (int, error) {
	if w.N > 0 {
		w.N--

		return w.Writer.Write(p)
	}

	return len(p), nil
}

func (w *HeadWriter) Unwrap() interface{} {
	return w.Writer
}

func NewSandwichWriter(filling io.Writer) *SandwichWriter {
	return &SandwichWriter{
		sandwichFilling: sandwichFilling{
			Writer: filling,
		},
	}
}

func (w *SandwichWriter) Write(p []byte) (int, error) {
	w.n = -1

	n, err := w.Writer.Write(p)

	if w.n != -1 {
		n = w.n
	}

	return n, err
}

func (w *sandwichFilling) Write(p []byte) (int, error) {
	n, err := w.Writer.Write(p)
	w.n = n

	return n, err
}

func (w *SandwichWriter) Inner() io.Writer {
	if c, ok := w.sandwichFilling.Writer.(io.Closer); ok {
		return WriteCloser{
			Writer: &w.sandwichFilling,
			Closer: c,
		}
	}

	return &w.sandwichFilling
}

func (w *SandwichWriter) Unwrap() interface{} {
	return w.Writer
}

func (w WriterFunc) Write(p []byte) (int, error) { return w(p) }

func WrapFlusherNoError(f FlusherNoError) Flusher {
	return wrapFlusher{FlusherNoError: f}
}

func (f wrapFlusher) Flush() error {
	f.FlusherNoError.Flush()
	return nil
}

func Fd(f interface{}) uintptr {
	const ffff = ^uintptr(0)

	if f == nil {
		return ffff
	}

	switch f := f.(type) {
	case interface{ Fd() uintptr }:
		return f.Fd()
	case interface{ Fd() int }:
		return uintptr(f.Fd())
	}

	return ffff
}

func WalkWriter(w io.Writer, f func(io.Writer) error) error {
	v, ok := w.(interface {
		WalkWriter(f func(io.Writer) error) error
	})
	if !ok {
		return nil
	}

	return v.WalkWriter(f)
}
