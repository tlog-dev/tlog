package tlwire

import (
	"testing"

	"github.com/nikandfor/assert"

	"github.com/nikandfor/tlog/low"
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

	SetEncoder(testEncoder{}, func(b []byte, val interface{}) []byte {
		return (Encoder{}).AppendInt(b, val.(testEncoder).N+1)
	})

	b = e.AppendValue(b[:0], testEncoder{N: 1})
	assert.Equal(t, low.Buf{Int | 2}, b)

	e.SetEncoder(testEncoder{}, func(b []byte, val interface{}) []byte {
		return (Encoder{}).AppendInt(b, val.(testEncoder).N+2)
	})

	b = e.AppendValue(b[:0], testEncoder{N: 1})
	assert.Equal(t, low.Buf{Int | 3}, b)

	b = (&Encoder{}).AppendValue(b[:0], testEncoder{N: 1})
	assert.Equal(t, low.Buf{Int | 2}, b)

	SetEncoder(testEncoder{}, nil)

	b = (&Encoder{}).AppendValue(b[:0], testEncoder{N: 1})
	assert.Equal(t, low.Buf{Int | 1}, b)
}
