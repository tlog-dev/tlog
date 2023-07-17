package tlog

import (
	"bytes"
	"context"
	"testing"

	"github.com/nikandfor/assert"
	"github.com/nikandfor/hacked/low"
)

func TestContextWithSpan(t *testing.T) {
	var buf, bufl low.Buf
	DefaultLogger = New(NewConsoleWriter(&buf, 0))
	DefaultLogger.NewID = testRandID(2)

	l := New(NewConsoleWriter(&bufl, 0))
	l.NewID = DefaultLogger.NewID

	id := ID{10, 20}

	tr := Span{Logger: DefaultLogger, ID: id}

	ctx := ContextWithSpan(context.Background(), tr)

	res := SpanFromContext(ctx)
	assert.Equal(t, tr, res)

	tr = SpawnFromContext(ctx, "spawn_1")
	if assert.NotZero(t, tr) {
		assert.Equal(t, "spawn_1                       _s=2f8282cb  _k=s  _p=0a140000\n", string(buf))
	}

	//
	ctx = ContextWithSpan(context.Background(), Span{})

	res = SpanFromContext(ctx)
	assert.Zero(t, res)

	//
	DefaultLogger = nil

	tr = Span{Logger: l, ID: id}

	ctx = ContextWithSpan(context.Background(), tr)

	res = SpanFromContext(ctx)
	assert.Equal(t, tr, res)

	tr = SpawnFromContext(ctx, "spawn_2")
	if assert.NotZero(t, tr) {
		assert.Equal(t, "spawn_2                       _s=d967dc28  _k=s  _p=0a140000\n", string(bufl))
	}
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
}
