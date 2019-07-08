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

func TestLocationZero(t *testing.T) {
	var l Location

	entry := l.Entry()
	assert.Equal(t, uintptr(0), entry)

	name, file, line := l.NameFileLine()
	assert.Equal(t, "", name)
	assert.Equal(t, "", file)
	assert.Equal(t, 0, line)
}
