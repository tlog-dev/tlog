package tlog

import (
	"bytes"
	"sync"

	"github.com/nikandfor/json"
)

type CircleBuffer struct {
	mu sync.Mutex
	l  [][]byte
	i  int
}

func NewCircleBuffer(n int) *CircleBuffer {
	return &CircleBuffer{
		l: make([][]byte, n),
	}
}

func (b *CircleBuffer) Write(p []byte) (int, error) {
	defer b.mu.Unlock()
	b.mu.Lock()

	l := b.l[b.i]

	if l == nil || cap(l) < len(p) {
		l = make([]byte, len(p))
		copy(l, p)
	} else {
		l = l[:len(p)]
		copy(l, p)
	}
	b.l[b.i] = l
	b.i = (b.i + 1) % len(b.l)

	return len(p), nil
}

func (b *CircleBuffer) MarshalJSON() ([]byte, error) {
	defer b.mu.Unlock()
	b.mu.Lock()

	w := json.NewWriter(nil)

	w.ArrayStart()

	i := b.i
	for {
		if b.l[i] != nil {
			l := b.l[i]
			last := len(l) - 1
			if l[last] != '\n' {
				last++
			}

			_, _ = w.Write(l[:last])
		}

		i = (i + 1) % len(b.l)

		if i == b.i {
			break
		}
	}

	w.ArrayEnd()

	if err := w.Err(); err != nil {
		return nil, err
	}

	return w.Bytes(), nil
}

func (b *CircleBuffer) MarshalText() ([]byte, error) {
	defer b.mu.Unlock()
	b.mu.Lock()

	var buf bytes.Buffer

	i := b.i
	for {
		if b.l[i] != nil {
			buf.Write(b.l[i])
		}

		i = (i + 1) % len(b.l)

		if i == b.i {
			break
		}
	}

	return buf.Bytes(), nil
}
