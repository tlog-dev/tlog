// TODO
// +build ignore

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
	DefaultLogger.NewID = testRandID()

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
		assert.Equal(t, `0194fdc2  Span started
6e4ff95f  Span spawned from 0a140000
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
	DefaultLogger.NewID = testRandID()

	l := New(NewConsoleWriter(&bufl, Lspans))
	l.NewID = DefaultLogger.NewID

	id := ID{10, 20}

	tr := Span{Logger: DefaultLogger, ID: id}

	ctx := ContextWithSpan(context.Background(), tr)

	trr := SpanFromContext(ctx)
	assert.Equal(t, id, trr.ID)

	res := IDFromContext(ctx)
	assert.Equal(t, id, res)

	tr = SpawnFromContext(ctx)
	if assert.NotZero(t, tr) {
		assert.Equal(t, "0194fdc2  Span spawned from 0a140000\n", string(buf))
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
		assert.Equal(t, "6e4ff95f  Span spawned from 0a140000\n", string(bufl))
	}

	bufl = bufl[:0]

	tr = SpawnFromContextOrStart(ctx)
	if assert.NotZero(t, tr) {
		assert.Equal(t, "fb180daf  Span spawned from 0a140000\n", string(bufl))
	}
}

func TestContextWithRandom(t *testing.T) {
	defer func(old *Logger) {
		DefaultLogger = old
	}(DefaultLogger)

	var buf bytes.Buffer
	DefaultLogger = New(NewConsoleWriter(&buf, Lspans))
	DefaultLogger.NewID = testRandID()

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
