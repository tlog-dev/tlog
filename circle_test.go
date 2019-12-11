package tlog

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCircleWrite(t *testing.T) {
	b := NewCircleBuffer(3)

	b.Write([]byte("message 1\n"))
	b.Write([]byte("message 2\n"))
	b.Write([]byte("message 3\n"))
	b.Write([]byte("message 1000\n"))
	b.Write([]byte("msg\n"))

	data, err := b.MarshalText()
	assert.NoError(t, err)
	assert.Equal(t, `message 3
message 1000
msg
`, string(data))
}

func TestCircleMarshalText(t *testing.T) {
	b := NewCircleBuffer(10)

	b.Write([]byte("message 1\n"))
	b.Write([]byte("message 2\n"))
	b.Write([]byte("message 3\n"))

	data, err := b.MarshalText()
	assert.NoError(t, err)
	assert.Equal(t, `message 1
message 2
message 3
`, string(data))
}

func TestCircleMarshalJSON(t *testing.T) {
	b := NewCircleBuffer(10)

	b.Write([]byte(`{"message":"1"}` + "\n"))
	b.Write([]byte(`{"message":"2"}`))
	b.Write([]byte(`{"message":"3"}` + "\n"))

	data, err := b.MarshalJSON()
	assert.NoError(t, err)
	assert.Equal(t, `[{"message":"1"},{"message":"2"},{"message":"3"}]`, string(data))
}
