package tlog

import (
	"testing"

	"github.com/nikandfor/tlog/low"
	"github.com/stretchr/testify/assert"
)

func TestDecoder(t *testing.T) {
	var b low.Buf
	var e Encoder

	kvs := []interface{}{"a", "b", "c", 3}

	b = e.AppendTag(b, Map, e.CalcMapLen(kvs))
	b = e.AppendKVs(b, kvs)

	kvs = []interface{}{"q", "w", "e", 4}

	b = e.AppendTag(b, Map, e.CalcMapLen(kvs))
	b = e.AppendKVs(b, kvs)

	var d Decoder
	d.ResetBytes(b)

	end := d.Skip(0)
	assert.Equal(t, int64(8), end)

	end = d.Skip(end)
	assert.Equal(t, int64(len(b)), end)

	t.Logf("dump\n%s", Dump(b))
}
