package tlog

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
	var l Frame

	entry := l.Entry()
	assert.Equal(t, Frame(0), entry)

	name, file, line := l.NameFileLine()
	assert.Equal(t, "", name)
	assert.Equal(t, "", file)
	assert.Equal(t, 0, line)
}

func TestStrHash(t *testing.T) {
	s1 := "str"
	s2 := "str"

	t.Logf("h(str1): %x", strhash(&s1, 0))
	t.Logf("h(str2): %x", strhash(&s2, 0))

	b1 := []byte(s1)
	b2 := []byte(s2)

	t.Logf("h(bytes1): %x", byteshash(&b1, 0))
	t.Logf("h(bytes2): %x", byteshash(&b2, 0))

	assert.Equal(t, strhash(&s1, 100), strhash(&s2, 100))
	assert.NotEqual(t, strhash(&s1, 1), strhash(&s2, 100))
}
