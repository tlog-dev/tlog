package low

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

func TestSafeAdd(t *testing.T) {
	t.Parallel()

	b := AppendSafeString(nil, `"\'`)
	assert.Equal(t, []byte(`\"\\'`), b)

	q := "\xbd\xb2\x3d\xbc\x20\xe2\x8c\x98"

	b = AppendQuoteString(nil, q)
	assert.Equal(t, []byte(`"\xbd\xb2=\xbc ⌘"`), b, "quoted: %q", q)

	if t.Failed() {
		i := 0
		for i < len(q) {
			r, w := utf8.DecodeRuneInString(q[i:])

			t.Logf("rune '%c' %8x  w %d\n", r, r, w)

			i += w
		}

		data, err := json.Marshal(q)

		t.Logf("json: << %s >>  err %v", data, err)
	}

	//	t.Logf("res: '%s'", w.Bytes())
}

func TestSafeMultiline(t *testing.T) {
	t.Parallel()

	const data = `flagfile: /etc/flags.flagfile
--friends ,
--clickhouse tcp://127.0.0.1:9000
--discovery=true
--debug :6061
`

	b := AppendSafeString(nil, data)

	//	var dec string

	assert.Equal(t, strings.ReplaceAll(data, "\n", "\\n"), string(b))
}
