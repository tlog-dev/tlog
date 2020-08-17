package tlog

import (
	"path/filepath"
	"reflect"
	"testing"
	"unsafe"
)

type testingWriter struct {
	t unsafe.Pointer
}

func newTestingWriter(t testing.TB) testingWriter {
	return testingWriter{t: unsafe.Pointer(reflect.ValueOf(t).Pointer())}
}

func (t testingWriter) Write(p []byte) (int, error) {
	l := Caller(4)
	_, file, line := l.NameFileLine()
	file = filepath.Base(file)

	pad := 0
	if len(file) < 20 {
		pad = 20 - len(file)
	}
	for line < 1000 {
		pad++
		line *= 10
	}

	padded := make([]byte, len(p)+pad)
	copy(padded[pad:], p)
	for i := 0; i < pad; i++ {
		padded[i] = ' '
	}

	testingLogDepth(t.t, string(padded), 5)

	return len(p), nil
}

//go:linkname testingLogDepth testing.(*common).logDepth
func testingLogDepth(t unsafe.Pointer, s string, d int)
