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
	var buf low.Buf
	e := Encoder{Writer: &buf}

	err := e.Encode(nil, []interface{}{"key", "value", "int_key", 4})
	assert.NoError(t, err)
	assert.Equal(t, `   0  a2  -  map: len 2
   1    63  -  "key"
   5    65  -  "value"
   b    67  -  "int_key"
  13    04  -  int          4
`, Dump(buf))

	buf = buf[:0]

	err = e.Encode(nil, []interface{}{Error, "error", errors.New("some error")})
	assert.NoError(t, err)
	assert.Equal(t, `   0  a2  -  map: len 2
   1    61  -  "i"
   3    c9  -  semantic  9
   4      02  -  int          2
   5    65  -  "error"
   b    c5  -  semantic  5
   c      6a  -  "some error"
`, Dump(buf))

	buf = buf[:0]

	err = e.Encode(nil, []interface{}{3, typedString("key"), "val"})
	assert.NoError(t, err)
	assert.Equal(t, `   0  a3  -  map: len 3
   1    6b  -  "MISSING_KEY"
   d    03  -  int          3
   e    6b  -  "MISSING_KEY"
  1a    63  -  "key"
  1e    63  -  "val"
  22    f7  -  undefined
`, Dump(buf))
}
