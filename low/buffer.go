package low

import (
	"sync"
	"unsafe"
)

type (
	Buf []byte

	Bwr struct {
		b   Buf
		buf [128 - unsafe.Sizeof([]byte{})]byte
	}
)

var bufPool = sync.Pool{New: func() interface{} { w := &Bwr{}; w.b = w.buf[:]; return w }}

var Spaces = []byte("                                                                                                                                                ")

// Getbuf gets bytes buffer from a pool to reduce gc pressure.
// buffer is at least 100 bytes long.
// Buffer must be returned after used. Usage:
//     b, wr := tlog.Getbuf()
//     defer wr.Ret(&b)
//
//     b = append(b[:0], ...)
func Getbuf() (_ Buf, wr *Bwr) { //nolint:golint
	wr = bufPool.Get().(*Bwr)
	return wr.b, wr
}

func (wr *Bwr) Ret(b *Buf) {
	wr.b = *b
	bufPool.Put(wr)
}

func (w *Buf) Write(p []byte) (int, error) {
	*w = append(*w, p...)

	return len(p), nil
}

func (w *Buf) NewLine() {
	l := len(*w)
	if l == 0 || (*w)[l-1] != '\n' {
		*w = append(*w, '\n')
	}
}

func UnsafeBytesToString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}
