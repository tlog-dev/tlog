package tlog

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

//line fifile.go:100
func TestLocation3(t *testing.T) {
	testInline(t)
}

func testInline(t *testing.T) {
	testLocation3(t)
}

func testLocation3(t *testing.T) {
	l := location(1)
	assert.Equal(t, "fifile.go:105", l.Short())
}
