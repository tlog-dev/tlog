package tlog

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContext(t *testing.T) {
	defer func(old *Logger) {
		DefaultLogger = old
	}(DefaultLogger)
	randID = testRandID()

	var buf bytes.Buffer
	DefaultLogger = New(NewConsoleWriter(&buf, Lspans))

	ctx := ContextWithID(context.Background(), z)
	tr := SpawnFromContext(ctx)
	assert.Zero(t, tr.ID)

	id := ID{10, 20}
	ctx = ContextWithID(context.Background(), id)

	res := IDFromContext(ctx)

	assert.Equal(t, id, res)

	tr = SpawnFromContext(ctx)
	if assert.NotNil(t, tr) {
		assert.Equal(t, `Span 0194fdc2fa2ffcc0 par 0a14000000000000 started`+"\n", buf.String())
	}

	DefaultLogger = nil

	tr = SpawnFromContext(ctx)
	assert.Zero(t, tr.ID)
}
