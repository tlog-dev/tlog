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

func (c *Buffer) Write(p []byte) (int, error) {
	defer c.mu.Unlock()
	c.mu.Lock()

	l := c.l[c.i]

	if l == nil || cap(l) < len(p) {
		l = make([]byte, len(p))
		copy(l, p)
	} else {
		l = l[:len(p)]
		copy(l, p)
	}
	c.l[c.i] = l
	c.i = (c.i + 1) % len(c.l)

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

func (c *Buffer) MarshalText() ([]byte, error) {
	defer c.mu.Unlock()
	c.mu.Lock()

	var buf bytes.Buffer

	i := c.i
	for {
		if c.l[i] != nil {
			buf.Write(c.l[i])
		}

		i = (i + 1) % len(c.l)

		if i == c.i {
			break
		}
	}

	return buf.Bytes(), nil
}
