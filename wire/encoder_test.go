package wire

import (
	"testing"

	"github.com/nikandfor/tlog/low"
	"gotest.tools/assert"
)

type A struct {
	A *A
	B int
	C string
}

func TestEncoder(t *testing.T) {
	var buf low.Buf

	d := NewDumper(&buf)
	d.NoGlobalOffset = true

	e := &Encoder{
		Writer: d,
	}

	q := &A{
		A: &A{
			A: &A{},
			B: 2,
			C: "a.c",
		},
		B: 1,
		C: "a",
	}

	Event(e, nil, []interface{}{"struct", q})

	assert.Equal(t, `   0  9f  -  array: len -1
   1    cc  -  semantic  c
   2      bf  -  object: len -1
   3        66  -  "struct"
   a        a3  -  object: len 3
   b          61  -  "A"
   d          a3  -  object: len 3
   e            61  -  "A"
  10            a3  -  object: len 3
  11              61  -  "A"
  13              f6  -  null
  14              61  -  "B"
  16              00  -  int          0
  17              61  -  "C"
  19              60  -  ""
  1a            61  -  "B"
  1c            02  -  int          2
  1d            61  -  "C"
  1f            63  -  "a.c"
  23          61  -  "B"
  25          01  -  int          1
  26          61  -  "C"
  28          61  -  "a"
  2a        ff  -  break
  2b    ff  -  break
`, string(buf))

	buf = buf[:0]
}
