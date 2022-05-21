//go:build ignore

package tq

import (
	"bytes"
	"io"
	"testing"

	"github.com/nikandfor/tlog/wire"
	"github.com/stretchr/testify/assert"
)

func TestKeys(t *testing.T) {
	var e wire.Encoder

	testFilter(t, func(b []byte) []byte {
		b = e.AppendMap(b, 3)
		b = e.AppendKeyInt(b, "i0", 2)
		b = e.AppendKeyString(b, "s0", "")
		b = e.AppendKeyString(b, "s4", "v123")

		return b
	}, func(b []byte) []byte {
		b = e.AppendArray(b, 3)
		b = e.AppendString(b, "i0")
		b = e.AppendString(b, "s0")
		b = e.AppendString(b, "s4")

		return b
	}, func(r io.Reader) io.Reader {
		return &Keys{State: wire.State{Reader: r}}
	})
}

func testFilter(t *testing.T, data, exp func(b []byte) []byte, filter func(r io.Reader) io.Reader) {
	b := data(nil)
	b = data(b)

	r := exp(nil)
	r = exp(r)

	f := filter(bytes.NewReader(b))

	var p []byte

	i := 0
	for {
		n, err := f.Read(p[i:])
		i += n
		if err == io.ErrShortBuffer {
			p = append(p, 0)
			continue
		}

		if err == io.EOF {
			break
		}

		assert.NoError(t, err)
	}

	assert.Equal(t, r, p[:i])

	t.Logf("reader: % x", b)
}
