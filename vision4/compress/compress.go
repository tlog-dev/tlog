package compress

import (
	"io"
	"unsafe"

	"github.com/nikandfor/tlog/low"
)

const (
	htSize = 0x8000
	htMask = htSize - 1
)

type (
	Writer struct {
		io.Writer

		p []byte

		i, bmask int
		b        []byte

		ht [htSize]int
	}
)

const (
	Literal = iota << 6
	Copy
	Entropy
	Meta
)

func (w *Writer) Write(p []byte) (st int, err error) {
	if w.b == nil {
		w.b = make([]byte, 0x10000)
		w.bmask = len(w.b) - 1

		w.p = append(w.p, Meta)
	}

	for i := st; i < len(p); {
		h := low.MemHash32(unsafe.Pointer(&p[i]), 0)

		ref := w.ht[h&htMask]

		if ref < w.i-len(w.b) {
			i += 2
			continue
		}

		c := w.common(ref, p[i:])

		if c < 4 {
			i += 2
			continue
		}

		// st to i is for Data
		// c bytes from i are copy of prev data at ref
	}

	return 0, nil
}

func (w *Writer) common(ref int, p []byte) (i int) {
	for ; i < len(p); i++ {
		if p[i] != w.b[(w.i+i)&w.bmask] {
			return
		}
	}

	return 0
}
