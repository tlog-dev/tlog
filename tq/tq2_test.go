//go:build ignore

package tq

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/low"
	"github.com/nikandfor/tlog/wire"
)

func TestTQ(t *testing.T) {
	testTQ(t, `keys`, []interface{}{rawArr(2), "a", "b"}, []interface{}{rawObj(2), "a", 1, "b", 2})
	testTQ(t, `keys | cat`, []interface{}{rawStr("ab")}, []interface{}{rawObj(2), "a", 1, "b", 2})
	testTQ(t, `"a"`, []interface{}{rawStr("a")}, nil)
	testTQ(t, `3`, []interface{}{rawInt(3)}, nil)
	testTQ(t, `3.6`, []interface{}{rawFlt(3.6)}, nil)
	testTQ(t, `length`, []interface{}{rawInt(3)}, []interface{}{rawStr("abc")})
	testTQ(t, `3 > 2`, []interface{}{rawSpec(wire.True)}, nil)
}

func rawSpec(x byte) tlog.RawMessage {
	var e wire.Encoder
	return e.AppendSpecial(nil, x)
}

func rawInt(x int) tlog.RawMessage {
	var e wire.Encoder
	return e.AppendInt(nil, x)
}

func rawFlt(x float64) tlog.RawMessage {
	var e wire.Encoder
	return e.AppendFloat(nil, x)
}

func rawStr(s string) tlog.RawMessage {
	var e wire.Encoder
	return e.AppendString(nil, s)
}

func rawArr(l int) tlog.RawMessage {
	return rawTag(nil, wire.Array, l)
}

func rawObj(l int) tlog.RawMessage {
	return rawTag(nil, wire.Map, l)
}

func rawBreak() tlog.RawMessage {
	return rawTag(nil, wire.Special, wire.Break)
}

func rawTag(b tlog.RawMessage, tag byte, sub int) tlog.RawMessage {
	var e wire.Encoder

	return e.AppendTag(b, tag, sub)
}

func testTQ(t *testing.T, q string, exp, data []interface{}) {
	var e wire.Encoder

	ee := tlog.AppendKVs(&e, nil, exp)
	in := tlog.AppendKVs(&e, nil, data)

	var b low.Buf

	f, err := New(&b, q)
	require.NoError(t, err)

	n, err := f.Write(in)
	assert.NoError(t, err)
	assert.Equal(t, len(ee), n)

	if !assert.Equal(t, ee, []byte(b)) {
		t.Logf("expected:\n%s", wire.Dump(ee))
		t.Logf("got:\n%s", wire.Dump(b))
	}
}
