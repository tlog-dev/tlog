package low

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type testError struct{}

func TestIsNil(t *testing.T) {
	var e interface{}
	var i = 3

	assert.Equal(t, true, IsNil(e))

	assert.Equal(t, false, IsNil(i))

	e = i

	assert.Equal(t, false, IsNil(e))

	e = (*int)(nil)

	assert.Equal(t, true, IsNil(e))

	var err error

	assert.Equal(t, true, IsNil(err))

	err = &testError{}

	assert.Equal(t, false, IsNil(err))

	err = (*testError)(nil)

	assert.Equal(t, true, IsNil(err))
}

func (*testError) Error() string { return "test error" }
