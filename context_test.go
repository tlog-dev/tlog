package tlog

import (
	"bytes"
	"context"
	"testing"

	"github.com/nikandfor/tlog/low"
	"github.com/stretchr/testify/assert"
)

func TestContextWithID(t *testing.T) {
	defer func(old *Logger) {
		DefaultLogger = old
	}(DefaultLogger)

	var buf bytes.Buffer
	DefaultLogger = New(NewConsoleWriter(&buf, 0))
	DefaultLogger.NewID = testRandID(1)

	ctx := ContextWithID(context.Background(), ID{})
	tr := SpawnFromContext(ctx, "spawn_1")
	assert.Zero(t, tr)

	tr = SpawnOrStartFromContext(ctx, "spawn_or_start_1")
	assert.NotZero(t, tr.ID)

	//
	id := ID{10, 20}
	ctx = ContextWithID(context.Background(), id)

	res := IDFromContext(ctx)

	assert.Equal(t, id, res)

	tr = SpawnFromContext(ctx, "spawn_2")
	if assert.NotZero(t, tr) {
		assert.Equal(t, `spawn_or_start_1              s=52fdfc07  K=s
spawn_2                       s=9566c74d  K=s  p=0a140000
`, buf.String())
	}

	//
	DefaultLogger = nil

	tr = SpawnFromContext(ctx, "spawn_3")
	assert.Zero(t, tr)

	tr = SpawnOrStartFromContext(ctx, "spawn_or_start_2")
	assert.Zero(t, tr)
}

func TestContextWithSpan(t *testing.T) {
	var buf, bufl low.Buf
	DefaultLogger = New(NewConsoleWriter(&buf, 0))
	DefaultLogger.NewID = testRandID(2)

	l := New(NewConsoleWriter(&bufl, 0))
	l.NewID = DefaultLogger.NewID

	id := ID{10, 20}

	tr := Span{Logger: DefaultLogger, ID: id}

	ctx := ContextWithSpan(context.Background(), tr)

	trr := SpanFromContext(ctx)
	assert.Equal(t, id, trr.ID)

	res := IDFromContext(ctx)
	assert.Equal(t, id, res)

	tr = SpawnFromContext(ctx, "spawn_1")
	if assert.NotZero(t, tr) {
		assert.Equal(t, "spawn_1                       s=2f8282cb  K=s  p=0a140000\n", string(buf))
	}

	//
	ctx = ContextWithID(context.Background(), id)

	tr = SpanFromContext(ctx)
	assert.Zero(t, tr)

	//
	ctx = ContextWithSpan(context.Background(), Span{})

	res = IDFromContext(ctx)
	assert.Zero(t, res)

	//
	DefaultLogger = nil

	tr = Span{Logger: l, ID: id}

	ctx = ContextWithSpan(context.Background(), tr)

	trr = SpanFromContext(ctx)
	assert.Equal(t, tr, trr)

	tr = SpawnFromContext(ctx, "spawn_2")
	if assert.NotZero(t, tr) {
		assert.Equal(t, "spawn_2                       s=d967dc28  K=s  p=0a140000\n", string(bufl))
	}

	bufl = bufl[:0]

	tr = SpawnOrStartFromContext(ctx, "spawn_or_start_1")
	if assert.NotZero(t, tr) {
		assert.Equal(t, "spawn_or_start_1              s=686ba0dc  K=s  p=0a140000\n", string(bufl))
	}
}

func TestContextWithRandom(t *testing.T) {
	defer func(old *Logger) {
		DefaultLogger = old
	}(DefaultLogger)

	var buf bytes.Buffer
	DefaultLogger = New(NewConsoleWriter(&buf, 0))
	DefaultLogger.NewID = testRandID(3)

	id := ID{10, 20}

	ctx := ContextWithIDOrRandom(context.Background(), id)
	res := IDFromContext(ctx)
	assert.Equal(t, id, res)

	ctx = ContextWithIDOrRandom(context.Background(), ID{})
	res = IDFromContext(ctx)
	assert.NotZero(t, res)
	assert.NotEqual(t, id, res)

	ctx = ContextWithRandomID(context.Background())
	res = IDFromContext(ctx)
	assert.NotZero(t, res)

	DefaultLogger = nil

	ctx = ContextWithRandomID(context.Background())
	assert.Equal(t, context.Background(), ctx)
}

func TestContextResetSpan(t *testing.T) {
	defer func(old *Logger) {
		DefaultLogger = old
	}(DefaultLogger)

	var buf bytes.Buffer
	DefaultLogger = New(NewConsoleWriter(&buf, 0))
	DefaultLogger.NewID = testRandID(3)

	tr := Start("root")

	ctx := ContextWithSpan(context.Background(), tr)

	//
	ctx2 := ContextWithSpan(ctx, tr.V("nope"))

	tr2 := SpawnFromContext(ctx2, "spawn")
	assert.Zero(t, tr2)

	t.Skip("not specified")

	//
	ctx2 = ContextWithID(ctx, (ID{}))

	tr2 = SpawnFromContext(ctx2, "spawn")
	assert.NotZero(t, tr2)

	//
	ctx2 = ContextWithLogger(ctx, (*Logger)(nil))

	tr2 = SpawnFromContext(ctx2, "spawn")
	assert.NotZero(t, tr2)
}
