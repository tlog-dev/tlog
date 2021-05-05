package tlog

import (
	"testing"

	"github.com/nikandfor/errors"
	"github.com/nikandfor/tlog/low"
	"github.com/stretchr/testify/assert"
)

type (
	typedString string
)

func TestEncoder(t *testing.T) {
	var b low.Buf
	var e Encoder

	kvs := []interface{}{"key", "value", "int_key", 4}
	ll := e.CalcMapLen(kvs)
	b = e.AppendTag(b, Map, ll)
	b = e.AppendKVs(b, kvs)
	assert.Equal(t, `   0  a2  -  map: len 2
   1    63  -  "key"
   5    65  -  "value"
   b    67  -  "int_key"
  13    04  -  int          4
`, Dump(b))

	b = b[:0]

	kvs = []interface{}{Error, "error", errors.New("some error")}
	ll = e.CalcMapLen(kvs)
	b = e.AppendTag(b, Map, ll)
	b = e.AppendKVs(b, kvs)
	assert.Equal(t, `   0  a2  -  map: len 2
   1    6b  -  "MISSING_KEY"
   d    c9  -  semantic  9
   e      02  -  int          2
   f    65  -  "error"
  15    c5  -  semantic  5
  16      6a  -  "some error"
`, Dump(b))

	b = b[:0]

	kvs = []interface{}{3, typedString("typed_str"), "val"}
	ll = e.CalcMapLen(kvs)
	b = e.AppendTag(b, Map, ll)
	b = e.AppendKVs(b, kvs)
	assert.Equal(t, `   0  a3  -  map: len 3
   1    6b  -  "MISSING_KEY"
   d    03  -  int          3
   e    6b  -  "MISSING_KEY"
  1a    69  -  "typed_str"
  24    63  -  "val"
  28    f7  -  undefined
`, Dump(b))
}
