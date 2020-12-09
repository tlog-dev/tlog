package tlog

import (
	"path/filepath"
	"reflect"
	"testing"
	"unsafe"

	"github.com/nikandfor/loc"
	"github.com/nikandfor/tlog/low"
)

type testingWriter struct {
	t unsafe.Pointer
	b []byte
}

func newTestingWriter(t testing.TB) testingWriter {
	return testingWriter{t: unsafe.Pointer(reflect.ValueOf(t).Pointer())}
}

func (t testingWriter) Write(p []byte) (int, error) {
	l := loc.Caller(5)
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

	b := t.b
	b = append(b, low.Spaces[:pad]...)
	b = append(b, p...)

	t.b = b[:0]

	testingLogDepth(t.t, low.UnsafeBytesToString(b), 6)

	return len(p), nil
}

//go:linkname testingLogDepth testing.(*common).logDepth
func testingLogDepth(t unsafe.Pointer, s string, d int)
