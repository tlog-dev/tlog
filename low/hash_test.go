package low

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStrHash(t *testing.T) {
	s1 := "str"
	s2 := "str"

	t.Logf("h(str1): %x", StrHash(s1, 0))
	t.Logf("h(str2): %x", StrHash(s2, 0))

	b1 := []byte(s1)
	b2 := []byte(s2)

	t.Logf("h(bytes1): %x", BytesHash(b1, 0))
	t.Logf("h(bytes2): %x", BytesHash(b2, 0))

	assert.Equal(t, StrHash(s1, 100), StrHash(s2, 100))
	assert.NotEqual(t, StrHash(s1, 1), StrHash(s2, 100))
}
