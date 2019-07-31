package tlog

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContext(t *testing.T) {
	defer func(old *Logger) {
		DefaultLogger = old
	}(DefaultLogger)

	ctx := ContextWithID(context.Background(), 0)
	tr := SpawnFromContext(ctx)
	assert.Nil(t, tr)

	id := ID(100)
	ctx = ContextWithID(context.Background(), id)

	res := IDFromContext(ctx)

	assert.Equal(t, id, res)

	tr = SpawnFromContext(ctx)
	if assert.NotNil(t, tr) {
		//assert.Equal(t, tr.Parent, id)
	}

	DefaultLogger = nil

	tr = SpawnFromContext(ctx)
	assert.Nil(t, tr)
}
