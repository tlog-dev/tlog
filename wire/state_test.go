package wire

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStateInt1(t *testing.T) {
	testState(t, func(b []byte) []byte {
		var e Encoder

		return e.AppendInt(b, 7)
	})
}

func TestStateInt2(t *testing.T) {
	testState(t, func(b []byte) []byte {
		var e Encoder

		return e.AppendInt(b, 10000000)
	})
}

func TestStateString1(t *testing.T) {
	testState(t, func(b []byte) []byte {
		var e Encoder

		return e.AppendString(b, "a")
	})
}

func TestStateString2(t *testing.T) {
	testState(t, func(b []byte) []byte {
		var e Encoder

		return e.AppendString(b, "1234567890")
	})
}

func TestStateArray1(t *testing.T) {
	testState(t, func(b []byte) []byte {
		var e Encoder

		b = e.AppendArray(b, 2)
		b = e.AppendInt(b, 7)
		b = e.AppendString(b, "v2")

		return b
	})
}

func TestStateArray2(t *testing.T) {
	testState(t, func(b []byte) []byte {
		var e Encoder

		b = e.AppendArray(b, -1)
		b = e.AppendInt(b, 7)
		b = e.AppendString(b, "v2")
		b = e.AppendBreak(b)

		return b
	})
}

func TestStateMap1(t *testing.T) {
	testState(t, func(b []byte) []byte {
		var e Encoder

		b = e.AppendMap(b, 5)
		b = e.AppendKeyInt(b, "i0", 2)
		b = e.AppendKeyInt(b, "i1", 100)
		b = e.AppendKeyString(b, "s2", "")
		b = e.AppendKeyString(b, "s3", "v")
		b = e.AppendKeyString(b, "s4", "v123")

		return b
	})
}

func TestStateMap2(t *testing.T) {
	testState(t, func(b []byte) []byte {
		var e Encoder

		b = e.AppendMap(b, -1)
		b = e.AppendKeyInt(b, "i0", 2)
		b = e.AppendKeyString(b, "s1", "v")
		b = e.AppendBreak(b)

		return b
	})
}

func TestStateSemantic1(t *testing.T) {
	testState(t, func(b []byte) []byte {
		var e Encoder

		b = e.AppendSemantic(b, Hex)
		b = e.AppendInt(b, 7)

		return b
	})
}

func TestStateSemantic2(t *testing.T) {
	testState(t, func(b []byte) []byte {
		var e Encoder

		b = e.AppendSemantic(b, 1000)
		b = e.AppendInt(b, 7)

		return b
	})
}

func TestStateSpecial1(t *testing.T) {
	testState(t, func(b []byte) []byte {
		var e Encoder

		b = e.AppendFloat(b, float64(float32(1.1234)))

		return b
	})
}

func TestStateSpecial2(t *testing.T) {
	testState(t, func(b []byte) []byte {
		var e Encoder

		b = e.AppendBreak(b)

		return b
	})
}

func TestStateTagMap2(t *testing.T) {
	testStateTag(t, func(b []byte) []byte {
		var e Encoder

		b = e.AppendMap(b, -1)
		b = e.AppendKeyInt(b, "i0", 2)
		b = e.AppendKeyString(b, "s1", "v")
		b = e.AppendBreak(b)

		return b
	})
}

func testState(t *testing.T, enc func(b []byte) []byte) {
	var b []byte

	b = enc(b[:0])

	var s State

	s.Reader = bytes.NewReader(b)

	var i int
	p := make([]byte, 1)

loop:
	for {
		n, err := s.Read(p[i:])
		t.Logf("read  i %x/%x  %x %v   s: %v  p % x %[6]q", i, len(p), n, err, s.s, p[i:i+n])
		i += n
		switch {
		case err == nil:
			break loop
		default:
			t.Errorf("error  i %x: %x %v", i, n, err)
			break loop
		case err == io.ErrShortBuffer:
			p = append(p, 0)
		}
	}

	assert.Equal(t, b, p[:i])

	//	if t.Failed() {
	t.Logf("reader: % x", b)
	//	}
}

func testStateTag(t *testing.T, enc func(b []byte) []byte) {
	var b []byte

	b = enc(b[:0])

	b = append(b, Neg|1)

	var s State

	s.Reader = bytes.NewReader(b)

	var n int
	p := make([]byte, 1)

	var ss *SubState
	var err error

loop:
	for {
		var m int

		ss, m, err = s.SubState(p[n:])
		t.Logf("read  n %x/%x  m %x %v   s: %v  p % x %[6]q", n, len(p), m, err, s.s, p[n:n+m])
		n += m
		switch {
		case err == nil:
			break loop
		case err == io.ErrShortBuffer:
			p = append(p, 0)
		default:
			t.Errorf("error  m %x: %x %v", m, n, err)
			break loop
		}
	}

	if t.Failed() {
		return
	}

	rv := func() error {
		for {
			m, err := ss.Read(p[n:])
			t.Logf("read  n %x/%x  m %x %v   s: %v  p % x %[6]q", n, len(p), m, err, s.s, p[n:n+m])
			n += m
			if err == io.ErrShortBuffer {
				p = append(p, 0)
				continue
			}

			return err
		}
	}

	switch ss.Tag {
	case Int, Neg:
	case Array, Map:
		for el := 0; ss.Sub == -1 || el < int(ss.Sub); el++ {
			if ss.Tag == Map {
				st := n
				err = rv()
				t.Logf("key  n %x/%x  => %v  el %x/%x  % x %[6]q", n, len(p), err, el, ss.Sub, p[st:n])
				if err == io.EOF {
					break
				}
				if !assert.NoError(t, err) {
					break
				}
			}

			st := n
			err = rv()
			t.Logf("val  n %x/%x  => %v  el %x/%x  % x %[6]q", n, len(p), err, el, ss.Sub, p[st:n])
			if !assert.NoError(t, err) {
				break
			}
		}
	default:
		t.Fatalf("unexpected tag: %x", ss.Tag)
	}

	st := n
	err = rv()
	t.Logf("after last value read: %v  % x  %[2]q", err, p[st:n])
	assert.Equal(t, io.EOF, err)
	assert.Equal(t, 0, n-st)

	err = ss.Close()
	assert.NoError(t, err)

	// read added trailer value

	p = append(p, 0)

	m, err := s.Read(p[n:])
	n += m
	assert.NoError(t, err)
	assert.Equal(t, 1, m)

	assert.Equal(t, b, p[:n])

	t.Logf("reader: % x", b)
}
