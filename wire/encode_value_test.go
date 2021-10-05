package wire

import (
	"errors"
	"testing"
	"time"

	"github.com/nikandfor/loc"
	"github.com/stretchr/testify/assert"
)

type Specials struct {
	Time     time.Time
	Duration time.Duration
	PC       loc.PC
	PCs      loc.PCs
	NilError error
	Error    error
}

func TestSpecials(t *testing.T) {
	var e Encoder
	var b []byte

	b = e.AppendValue(b[:0], time.Unix(0, 100))
	assert.Equal(t, []byte{Semantic | Time, Int | Len1, 100}, b, Dump(b))

	b = e.AppendValue(b[:0], time.Second)
	assert.Equal(t, []byte{Semantic | Duration, Int | Len4, 0x3b, 0x9a, 0xca, 0x0}, b, Dump(b))

	b = e.AppendValue(b[:0], &Specials{
		Time:     time.Unix(0, 100),
		Duration: time.Second,
		Error:    errors.New("some"),
	})
	assert.Equal(t, `   0  bf  -  map: len -1
   1    64  -  "Time"
   6    c2  -  semantic  2
   7      18 64  -  int        100
   9    68  -  "Duration"
  12    c3  -  semantic  3
  13      1a 3b 9a ca 00  -  int 1000000000
  18    62  -  "PC"
  1b    c4  -  semantic  4
  1c      f6  -  null
  1d    63  -  "PCs"
  21    c4  -  semantic  4
  22      80  -  array: len 0
  23    68  -  "NilError"
  2c    f6  -  null
  2d    65  -  "Error"
  33    c1  -  semantic  1
  34      64  -  "some"
  39    ff  -  break
  3a
`, Dump(b))
}

func TestEncodeBytes(t *testing.T) {
	var e Encoder

	v := &struct {
		Q [4]byte
		W []byte
	}{
		Q: [4]byte{1, 2, 3, 4},
		W: []byte{4, 3, 2, 1},
	}

	b := e.AppendValue(nil, v)

	assert.Equal(t, `   0  bf  -  map: len -1
   1    61  -  "Q"
   3    44  -  01 02 03 04
   8    61  -  "W"
   a    44  -  04 03 02 01
   f    ff  -  break
  10
`, Dump(b))
}

type S struct {
	A1    A1
	A1ptr *A1
	A2    A2
	A2ptr *A2
}
type A1 int
type A2 int

func (a A1) TlogAppend(e *Encoder, b []byte) []byte {
	b = e.AppendMap(b, 1)

	b = e.AppendKeyInt(b, "a", int64(a))

	return b
}

func (a *A2) TlogAppend(e *Encoder, b []byte) []byte {
	b = e.AppendMap(b, 1)

	b = e.AppendKeyInt(b, "b", int64(*a))

	return b
}

func TestAppenderCheck(t *testing.T) {
	var e Encoder

	s := &S{
		A1: 1,
		A2: 2,
	}

	s.A1ptr = &s.A1
	s.A2ptr = &s.A2

	b := e.AppendValue(nil, s)

	assert.Equal(t, `   0  bf  -  map: len -1
   1    62  -  "A1"
   4    a1  -  map: len 1
   5      61  -  "a"
   7      01  -  int          1
   8    65  -  "A1ptr"
   e    a1  -  map: len 1
   f      61  -  "a"
  11      01  -  int          1
  12    62  -  "A2"
  15    a1  -  map: len 1
  16      61  -  "b"
  18      02  -  int          2
  19    65  -  "A2ptr"
  1f    a1  -  map: len 1
  20      61  -  "b"
  22      02  -  int          2
  23    ff  -  break
  24
`, Dump(b), "%#v", s)
}
