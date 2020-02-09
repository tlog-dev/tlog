package circle

import (
	"bytes"
	"sync"
)

type Buffer struct {
	mu sync.Mutex
	l  [][]byte
	i  int
}

func NewBuffer(n int) *Buffer {
	return &Buffer{
		l: make([][]byte, n),
	}
}

func (b *Buffer) Write(p []byte) (int, error) {
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

func (c *Buffer) MarshalJSON() ([]byte, error) {
	defer c.mu.Unlock()
	c.mu.Lock()

	var b []byte
	b = append(b, '[')

	i := c.i
	for {
		if c.l[i] != nil {
			l := c.l[i]
			last := len(l) - 1
			if l[last] != '\n' {
				last++
			}

			if len(b) != 1 {
				b = append(b, ',')
			}

			b = append(b, l[:last]...)
		}

		i = (i + 1) % len(c.l)

		if i == c.i {
			break
		}
	}

	b = append(b, ']')

	return b, nil
}

func (b *Buffer) MarshalText() ([]byte, error) {
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
