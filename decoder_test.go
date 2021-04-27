package tlog

import (
	"testing"

	"github.com/nikandfor/tlog/low"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecoder(t *testing.T) {
	var b low.Buf
	e := Encoder{Writer: &b}

	err := e.Encode(nil, []interface{}{"a", "b", "c", 3})
	require.NoError(t, err)

	err = e.Encode(nil, []interface{}{"q", "w", "e", 4})
	require.NoError(t, err)

	var d Decoder
	d.ResetBytes(b)

	end := d.Skip(0)
	assert.Equal(t, int64(8), end)

	end = d.Skip(end)
	assert.Equal(t, int64(len(b)), end)

	t.Logf("dump\n%s", Dump(b))
}
