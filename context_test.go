package tlog

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContextWithID(t *testing.T) {
	defer func(old *Logger) {
		DefaultLogger = old
	}(DefaultLogger)
	randID = testRandID()

	var buf bytes.Buffer
	DefaultLogger = New(NewConsoleWriter(&buf, Lspans))

	ctx := ContextWithID(context.Background(), z)
	tr := SpawnFromContext(ctx)
	assert.Zero(t, tr.ID)

	//
	id := ID{10, 20}
	ctx = ContextWithID(context.Background(), id)

	res := IDFromContext(ctx)

	assert.Equal(t, id, res)

	tr = SpawnFromContext(ctx)
	if assert.NotZero(t, tr) {
		assert.Equal(t, `Span 0194fdc2fa2ffcc0 par 0a14000000000000 started`+"\n", buf.String())
	}

	//
	DefaultLogger = nil

	tr = SpawnFromContext(ctx)
	assert.Zero(t, tr.ID)
}

func TestContextWithSpan(t *testing.T) {
	defer func(old *Logger) {
		DefaultLogger = old
	}(DefaultLogger)
	randID = testRandID()

	var buf bytes.Buffer
	DefaultLogger = New(NewConsoleWriter(&buf, Lspans))

	id := ID{10, 20}

	ctx := ContextWithSpan(context.Background(), Span{ID: id})

	tr := SpanFromContext(ctx)
	assert.Equal(t, id, tr.ID)

	res := IDFromContext(ctx)
	assert.Equal(t, id, res)

	tr = SpawnFromContext(ctx)
	if assert.NotZero(t, tr) {
		assert.Equal(t, `Span 0194fdc2fa2ffcc0 par 0a14000000000000 started`+"\n", buf.String())
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
	ctx = ContextWithSpan(context.Background(), Span{ID: id})
	tr = SpanFromContext(ctx)
	assert.Zero(t, tr)
}

func TestContextWithRandom(t *testing.T) {
	defer func(old *Logger) {
		DefaultLogger = old
	}(DefaultLogger)
	randID = testRandID()

	var buf bytes.Buffer
	DefaultLogger = New(NewConsoleWriter(&buf, Lspans))

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
}
