package loc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLocation3(t *testing.T) {
	testInline(t)
}

func testInline(t *testing.T) {
	testLocation3(t)
}

func testLocation3(t *testing.T) {
	l := Caller(1)
	assert.Equal(t, "unsafe_test.go:14", l.String())
}

func TestLocationZero(t *testing.T) {
	var l PC

	entry := l.Entry()
	assert.Equal(t, PC(0), entry)

	name, file, line := l.NameFileLine()
	assert.Equal(t, "", name)
	assert.Equal(t, "", file)
	assert.Equal(t, 0, line)
}
