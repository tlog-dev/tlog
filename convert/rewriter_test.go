package convert

import (
	"testing"
	"time"

	"github.com/nikandfor/assert"
	"github.com/nikandfor/loc"

	"tlog.app/go/tlog"
	"tlog.app/go/tlog/low"
	"tlog.app/go/tlog/tlwire"
)

func TestRewriter(t *testing.T) {
	var obj, b low.Buf
	rew := NewRewriter(&b)
	rew.Rule = RewriterFunc(func(b, p []byte, path []tlog.RawMessage, kst, st int) ([]byte, int, error) {
		if st == len(p) {
			t.Logf("path %x  nil message", path)
			return nil, 0, ErrFallback
		}

		var k []byte
		if kst != -1 {
			k, _ = rew.Bytes(p, kst)
		}

		tag, sub, _ := rew.Tag(p, st)

		t.Logf("path %q  k %q  st %v  tag %x %x", path, k, st, tag, sub)

		return b, st, ErrFallback
	})

	t.Logf("empty")

	n, err := rew.Write(obj)
	assert.NoError(t, err)
	assert.Equal(t, len(obj), n)
	assert.Equal(t, obj, b)

	b = b[:0]
	obj = tlog.AppendKVs(obj[:0], []interface{}{
		tlog.RawTag(tlwire.Map, -1),
		"a", "b",
		"d", tlog.NextIs(tlwire.Duration), 1,
		tlog.Break,
	})

	t.Logf("case1")

	n, err = rew.Write(obj)
	assert.NoError(t, err)
	assert.Equal(t, len(obj), n)
	if !assert.Equal(t, obj, b) {
		t.Logf("%s", tlwire.Dump(b))
	}
}

func TestKeyRewriter(t *testing.T) {
	var obj, exp, b low.Buf

	rew := NewRewriter(&b)
	ren := NewKeyRenamer(nil,
		RenameRule{
			Path: []tlog.RawMessage{
				rew.AppendString(nil, tlog.KeyTimestamp),
				tlog.RawTag(tlwire.Semantic, tlwire.Time),
			},
			Rename: []byte("time"),
		},
		RenameRule{
			Path: []tlog.RawMessage{
				rew.AppendString(nil, tlog.KeyCaller),
				tlog.RawTag(tlwire.Semantic, tlwire.Caller),
			},
			Remove: true,
		},
	)
	rew.Rule = RewriterFunc(func(b, p []byte, path []tlog.RawMessage, kst, st int) ([]byte, int, error) {
		r, i, err := ren.Rewrite(b, p, path, kst, st)

		t.Logf("rename  %q %x %x -> %x %v\n%s", path, kst, st, i, err, tlwire.Dump(r[len(b):]))

		return r, i, err
	})

	t.Logf("empty")

	n, err := rew.Write(obj)
	assert.NoError(t, err)
	assert.Equal(t, len(obj), n)
	assert.Equal(t, obj, b)

	b = b[:0]
	obj = tlog.AppendKVs(obj[:0], []interface{}{
		tlog.RawTag(tlwire.Map, -1),
		tlog.KeyTimestamp, time.Unix(100000000, 0),
		tlog.KeyCaller, loc.Caller(0),
		tlog.Break,
	})

	exp = tlog.AppendKVs(exp[:0], []interface{}{
		tlog.RawTag(tlwire.Map, -1),
		"time", time.Unix(100000000, 0),
		tlog.Break,
	})

	t.Logf("case 1")

	n, err = rew.Write(obj)
	assert.NoError(t, err)
	assert.Equal(t, len(obj), n)
	if !assert.Equal(t, exp, b) {
		t.Logf("obj\n%s", tlwire.Dump(obj))
		t.Logf("exp\n%s", tlwire.Dump(exp))
		t.Logf("buf\n%s", tlwire.Dump(b))
	}
}
