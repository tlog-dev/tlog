package low

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
