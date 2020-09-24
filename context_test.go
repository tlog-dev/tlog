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

	var buf bytes.Buffer
	DefaultLogger = New(NewConsoleWriter(&buf, Lspans))
	DefaultLogger.randID = testRandID()

	ctx := ContextWithID(context.Background(), ID{})
	tr := SpawnFromContext(ctx)
	assert.Zero(t, tr)

	tr = SpawnFromContextOrStart(ctx)
	assert.NotZero(t, tr.ID)

	//
	id := ID{10, 20}
	ctx = ContextWithID(context.Background(), id)

	res := IDFromContext(ctx)

	assert.Equal(t, id, res)

	tr = SpawnFromContext(ctx)
	if assert.NotZero(t, tr) {
		assert.Equal(t, `0194fdc2fa2ffcc0  Span started
6e4ff95ff662a5ee  Span spawned from 0a14000000000000
`, buf.String())
	}

	//
	DefaultLogger = nil

	tr = SpawnFromContext(ctx)
	assert.Zero(t, tr)

	tr = SpawnFromContextOrStart(ctx)
	assert.Zero(t, tr)
}

func TestContextWithSpan(t *testing.T) {
	var buf, bufl bufWriter
	DefaultLogger = New(NewConsoleWriter(&buf, Lspans))
	DefaultLogger.randID = testRandID()

	l := New(NewConsoleWriter(&bufl, Lspans))
	l.randID = DefaultLogger.randID

	id := ID{10, 20}

	tr := Span{Logger: DefaultLogger, ID: id}

	ctx := ContextWithSpan(context.Background(), tr)

	trr := SpanFromContext(ctx)
	assert.Equal(t, id, trr.ID)

	res := IDFromContext(ctx)
	assert.Equal(t, id, res)

	tr = SpawnFromContext(ctx)
	if assert.NotZero(t, tr) {
		assert.Equal(t, "0194fdc2fa2ffcc0  Span spawned from 0a14000000000000\n", string(buf))
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

	tr = SpawnFromContext(ctx)
	if assert.NotZero(t, tr) {
		assert.Equal(t, "6e4ff95ff662a5ee  Span spawned from 0a14000000000000\n", string(bufl))
	}

	bufl = bufl[:0]

	tr = SpawnFromContextOrStart(ctx)
	if assert.NotZero(t, tr) {
		assert.Equal(t, "fb180daf48a79ee0  Span spawned from 0a14000000000000\n", string(bufl))
	}
}

func TestContextWithRandom(t *testing.T) {
	defer func(old *Logger) {
		DefaultLogger = old
	}(DefaultLogger)

	var buf bytes.Buffer
	DefaultLogger = New(NewConsoleWriter(&buf, Lspans))
	DefaultLogger.randID = testRandID()

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
