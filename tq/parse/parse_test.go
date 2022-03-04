package parse

import (
	"context"
	"testing"

	"github.com/nikandfor/tlog/low"
	"github.com/nikandfor/tlog/wire"

	"github.com/stretchr/testify/assert"
)

func TestParse(t *testing.T) {
	var b low.Buf
	var e wire.Encoder

	b = e.AppendMap(b, -1)

	b = e.AppendKey(b, "t")
	b = e.AppendTimestamp(b, 1000000000)

	b = e.AppendKey(b, "m")
	b = e.AppendString(b, "message")

	b = e.AppendBreak(b)

	x, i, err := Value{}.Parse(context.Background(), b, 0)
	assert.NoError(t, err)
	assert.Equal(t, len(b), i)

	assert.Equal(t, Map{
		KV{K: String("t"), V: Semantic{Code: wire.Time, V: Int{0x1a, 0x3b, 0x9a, 0xca, 0x00}}},
		KV{K: String("m"), V: String("message")},
	}, x)
}
