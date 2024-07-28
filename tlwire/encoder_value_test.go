package tlwire

import (
	"testing"

	"github.com/nikandfor/assert"
	"nikand.dev/go/hacked/low"
)

type testEncoder struct {
	N int
}

func (t testEncoder) TlogAppend(b []byte) []byte {
	return (Encoder{}).AppendInt(b, t.N)
}

func TestEncoderValueCustomEncoders(t *testing.T) {
	var b low.Buf
	var e Encoder

	b = e.AppendValue(b[:0], testEncoder{N: 1})
	assert.Equal(t, low.Buf{Int | 1}, b)

	SetEncoder(testEncoder{}, func(e *Encoder, b []byte, val interface{}) []byte {
		return e.AppendInt(b, val.(testEncoder).N+1)
	})

	b = e.AppendValue(b[:0], testEncoder{N: 1})
	assert.Equal(t, low.Buf{Int | 2}, b)

	e.SetEncoder(testEncoder{}, func(e *Encoder, b []byte, val interface{}) []byte {
		return e.AppendInt(b, val.(testEncoder).N+2)
	})

	b = e.AppendValue(b[:0], testEncoder{N: 1})
	assert.Equal(t, low.Buf{Int | 3}, b)

	b = (&Encoder{}).AppendValue(b[:0], testEncoder{N: 1})
	assert.Equal(t, low.Buf{Int | 2}, b)

	SetEncoder(testEncoder{}, nil)

	b = (&Encoder{}).AppendValue(b[:0], testEncoder{N: 1})
	assert.Equal(t, low.Buf{Int | 1}, b)
}
