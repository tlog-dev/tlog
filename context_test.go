package tlog

import (
	"bytes"
	"context"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContext(t *testing.T) {
	defer func(old *Logger) {
		DefaultLogger = old
	}(DefaultLogger)
	rnd = rand.New(rand.NewSource(0))
	var buf bytes.Buffer
	DefaultLogger = New(NewConsoleWriter(&buf, Lspans))

	ctx := ContextWithID(context.Background(), 0)
	tr := SpawnFromContext(ctx)
	assert.Zero(t, tr.ID)

	id := ID(100)
	ctx = ContextWithID(context.Background(), id)

	res := IDFromContext(ctx)

	assert.Equal(t, id, res)

	tr = SpawnFromContext(ctx)
	if assert.NotNil(t, tr) {
		assert.Equal(t, `Span 78fc2ffac2fd9401 par 0000000000000064 started`+"\n", buf.String())
	}

	DefaultLogger = nil

	tr = SpawnFromContext(ctx)
	assert.Zero(t, tr.ID)
}
