package wire

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
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
  1b    c5  -  semantic  5
  1c      f6  -  null
  1d    63  -  "PCs"
  21    c5  -  semantic  5
  22      80  -  array: len 0
  23    68  -  "NilError"
  2c    c1  -  semantic  1
  2d      f6  -  null
  2e    65  -  "Error"
  34    c1  -  semantic  1
  35      64  -  "some"
  3a    ff  -  break
  3b
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

func TestBig(t *testing.T) {
	var b []byte
	var e Encoder

	b = e.AppendValue(b, big.NewInt(123123123))

	b = e.AppendValue(b, big.NewRat(12, 123123123))

	b = e.AppendValue(b, big.NewFloat(0.123123123456456))

	t.Logf("%s", Dump(b))

	var d Decoder

	assert.Equal(t, BigInt, d.BigWhich(b, 0))

	var x1 big.Int
	raw, i := d.BigInt(b, 0, &x1)
	assert.Equal(t, []byte{0x7, 0x56, 0xb5, 0xb3}, raw)
	assert.Equal(t, big.NewInt(123123123), &x1)

	assert.Equal(t, BigRat, d.BigWhich(b, i))

	var x2 big.Rat
	raw, raw2, i := d.BigRat(b, i, &x2)
	assert.Equal(t, []byte{0x04}, raw)
	assert.Equal(t, []byte{0x2, 0x72, 0x3c, 0x91}, raw2)
	assert.Equal(t, big.NewRat(12, 123123123), &x2)

	assert.Equal(t, BigFloat, d.BigWhich(b, i))

	var x3 big.Float
	raw, i = d.BigFloat(b, i, &x3)
	assert.Equal(t, []byte("0.123123123456456"), raw)
	assert.Equal(t, big.NewFloat(0.123123123456456).String(), x3.String())

	assert.Equal(t, len(b), i)

}

func TestTypeSwitch(t *testing.T) {
	q := 0

	if _, ok := ((interface{})(time.Time{})).(fmt.Stringer); ok {
		t.Logf("time.Time is fmt.Stringer")
		q++
	}

	if _, ok := ((interface{})(&time.Time{})).(fmt.Stringer); ok {
		t.Logf("*time.Time is fmt.Stringer")
		q++
	}

	if _, ok := ((interface{})(big.Int{})).(fmt.Stringer); !ok {
		t.Logf("big.Int is NOT fmt.Stringer")
		q++
	}

	if _, ok := ((interface{})(&big.Int{})).(fmt.Stringer); ok {
		t.Logf("*big.Int is fmt.Stringer")
		q++
	}

	if q != 4 {
		t.Fail()
	}
}

type benchVal struct {
	Time  time.Time
	Time2 *time.Time

	BigInt  big.Int
	BigInt2 *big.Int

	BigFloat  big.Float
	BigFloat2 *big.Float

	PC loc.PC

	Err    error
	Err2   error
	Interf interface{}

	Str  strings.Builder
	Str2 *strings.Builder
}

func mkTestRaw() benchVal {
	v := benchVal{
		Time: time.Now(),
		PC:   loc.Caller(0),
		Err:  errors.New("error"),
	}

	v.Time2 = &v.Time

	//	v.BigInt.SetInt64(1000000000)
	//	v.BigInt2 = &v.BigInt

	//	v.BigFloat.SetInt64(1000000000)
	//	v.BigFloat2 = &v.BigFloat

	v.Str.WriteString("string builder")
	v.Str2 = &strings.Builder{}
	v.Str2.WriteString("second stringer")

	return v
}

func TestAppendRawTestStruct(t *testing.T) {
	var e Encoder

	v := mkTestRaw()

	buf := e.AppendValue(nil, &v)

	t.Logf("%s", Dump(buf))
}

func BenchmarkAppendRawInt(b *testing.B) {
	b.ReportAllocs()

	var buf []byte
	var e Encoder

	for i := 0; i < b.N; i++ {
		buf = e.AppendValue(buf[:0], 1)
	}
}

func BenchmarkAppendRawInt64(b *testing.B) {
	b.ReportAllocs()

	var buf []byte
	var e Encoder

	for i := 0; i < b.N; i++ {
		buf = e.AppendValue(buf[:0], int64(1))
	}
}

func BenchmarkAppendRawString(b *testing.B) {
	b.ReportAllocs()

	var buf []byte
	var e Encoder

	type S string

	for i := 0; i < b.N; i++ {
		buf = e.AppendValue(buf[:0], S("qweqwe"))
	}
}

func BenchmarkAppendRawTime(b *testing.B) {
	b.ReportAllocs()

	var buf []byte
	var e Encoder

	tm := time.Now()

	for i := 0; i < b.N; i++ {
		buf = e.AppendValue(buf[:0], tm)
	}
}

func BenchmarkAppendRawTimePtr(b *testing.B) {
	b.ReportAllocs()

	var buf []byte
	var e Encoder

	tm := time.Now()

	for i := 0; i < b.N; i++ {
		buf = e.AppendValue(buf[:0], &tm)
	}
}
