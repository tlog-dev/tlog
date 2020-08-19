package tlargs

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/nikandfor/tlog"
)

func TestIfVArg(t *testing.T) {
	l := tlog.New(tlog.Discard{})

	l.SetFilter("enabled")

	assert.Equal(t, 1, If(true, 1, 2))
	assert.Equal(t, 2, If(false, 1, 2))

	assert.Equal(t, 1, IfV(l, "enabled", 1, 2))
	assert.Equal(t, 2, IfV(l, "disabled", 1, 2))
	assert.Equal(t, 2, IfV(nil, "enabled", 1, 2))
}
