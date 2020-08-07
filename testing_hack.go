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
	v := reflect.ValueOf(t).Pointer()
	return testingWriter{t: unsafe.Pointer(v)}
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

	p = append(p, "                                     "[:pad]...)
	copy(p[pad:], p)
	for i := range p[:pad] {
		p[i] = ' '
	}

	testingLogDepth(t.t, string(p), 5)

	return len(p), nil
}

//go:linkname testingLogDepth testing.(*common).logDepth
func testingLogDepth(t unsafe.Pointer, s string, d int)
