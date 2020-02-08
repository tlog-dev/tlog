package tlog

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSafeAdd(t *testing.T) {
	b := appendSafe(nil, `"\'`)
	assert.Equal(t, []byte(`\"\\'`), b)

	q := "\xbd\xb2\x3d\xbc\x20\xe2\x8c\x98"

	b = appendSafe(nil, q)
	assert.Equal(t, []byte(`\xbd\xb2=\u003d\xbc \u0020\u2318`), b)

	//	t.Logf("res: '%s'", w.Bytes())
}
