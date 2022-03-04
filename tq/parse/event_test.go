package parse

import (
	"context"
	"testing"
	"time"
	"unsafe"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/low"
	"github.com/nikandfor/tlog/wire"

	"github.com/nikandfor/assert"
)

func TestEvent(t *testing.T) {
	t.Logf("event size: 0x%x (%[1]v bytes, %v words)", unsafe.Sizeof(Event{}), unsafe.Sizeof(Event{})/unsafe.Sizeof(int(0)))

	tm := time.Now()
	id := tlog.MathRandID()

	var b low.Buf

	b = event(b, tlog.KeyTime, tm, tlog.KeySpan, id, "a", "b", tlog.KeyLabels, tlog.Labels{"a=b", "c"})

	var p Event

	x, i, err := p.Parse(context.Background(), b, 0)
	assert.NoError(t, err)
	assert.Equal(t, len(b), i)
	assert.Equal(t, &Event{
		Timestamp: tm.UnixNano(),
		Spans:     []tlog.ID{id},
		KVs: []LazyKV{
			{K: String("a"), V: String("b").TlogAppend(&wire.Encoder{}, nil)},
		},
		Labels: tlog.Labels{"a=b", "c"}.TlogAppend(&wire.Encoder{}, nil),
	}, x)

	if t.Failed() {
		t.Logf("dump\n%s", wire.Dump(b))
	}
}

func BenchmarkEventParse(b *testing.B) {
	b.ReportAllocs()

	tm := time.Now()
	id := tlog.MathRandID()

	raw := event(nil, tlog.KeyTime, tm, tlog.KeySpan, id, "a", "b", tlog.KeyLabels, tlog.Labels{"a=b", "c"})

	ctx := context.Background()

	for i := 0; i < b.N; i++ {
		var p Event
		_, _, _ = p.Parse(ctx, raw, 0)
	}
}

func event(b []byte, kvs ...interface{}) []byte {
	var e wire.Encoder

	b = e.AppendMap(b[:0], -1)

	b = tlog.AppendKVs(&e, b, kvs)

	b = e.AppendBreak(b)

	return b
}
