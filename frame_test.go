package tlog

import (
	"bytes"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFrame(t *testing.T) {
	testFrameInside(t)
}

func testFrameInside(t *testing.T) {
	pc := Caller(0)
	name, file, line := pc.NameFileLine()
	assert.Equal(t, "tlog.testFrameInside", path.Base(name))
	assert.Equal(t, "frame_test.go", filepath.Base(file))
	assert.Equal(t, 19, line)
}

func TestFrameShort(t *testing.T) {
	pc := Caller(0)
	assert.Equal(t, "frame_test.go:27", pc.String())
}

func TestFrame2(t *testing.T) {
	func() {
		func() {
			l := Funcentry(0)

			assert.Equal(t, "frame_test.go:33", l.String())
		}()
	}()
}

func TestFrameFormat(t *testing.T) {
	l := Caller(-1)

	var b bytes.Buffer

	fmt.Fprintf(&b, "%v", l)
	assert.Equal(t, "frame.go:25", b.String())

	b.Reset()

	fmt.Fprintf(&b, "%.3v", l)
	assert.Equal(t, "frame.go: 25", b.String())

	b.Reset()

	fmt.Fprintf(&b, "%15.3v", l)
	assert.Equal(t, "frame.go   : 25", b.String())

	b.Reset()

	fmt.Fprintf(&b, "%+v", l)
	assert.True(t, regexp.MustCompile(`[\w./-]*frame.go:25`).MatchString(b.String()))

	b.Reset()

	fmt.Fprintf(&b, "%#v", l)
	assert.Equal(t, "Caller:25", b.String())
}

func TestFrameCropFileName(t *testing.T) {
	assert.Equal(t, "github.com/nikandfor/tlog/sub/module/file.go",
		cropFilename("/path/to/src/github.com/nikandfor/tlog/sub/module/file.go", "github.com/nikandfor/tlog/sub/module.(*type).method"))
	assert.Equal(t, "github.com/nikandfor/tlog/sub/module/file.go",
		cropFilename("/path/to/src/github.com/nikandfor/tlog/sub/module/file.go", "github.com/nikandfor/tlog/sub/module.method"))
	assert.Equal(t, "github.com/nikandfor/tlog/root.go", cropFilename("/path/to/src/github.com/nikandfor/tlog/root.go", "github.com/nikandfor/tlog.type.method"))
	assert.Equal(t, "github.com/nikandfor/tlog/root.go", cropFilename("/path/to/src/github.com/nikandfor/tlog/root.go", "github.com/nikandfor/tlog.method"))
	assert.Equal(t, "root.go", cropFilename("/path/to/src/root.go", "github.com/nikandfor/tlog.method"))
	assert.Equal(t, "sub/file.go", cropFilename("/path/to/src/sub/file.go", "github.com/nikandfor/tlog/sub.method"))
	assert.Equal(t, "root.go", cropFilename("/path/to/src/root.go", "tlog.method"))
	assert.Equal(t, "subpkg/file.go", cropFilename("/path/to/src/subpkg/file.go", "subpkg.method"))
	assert.Equal(t, "subpkg/file.go", cropFilename("/path/to/src/subpkg/file.go", "github.com/nikandfor/tlog/subpkg.(*type).method"))
}

func TestCaller(t *testing.T) {
	a, b := Caller(0),
		Caller(0)

	assert.False(t, a == b, "%x == %x", uintptr(a), uintptr(b))
}

func TestSetCache(t *testing.T) {
	l := Frame(0x1234567890)

	assert.NotEqual(t, "file.go:10", l.String())

	l.SetCache("Name", "file.go", 10)

	assert.Equal(t, "file.go:10", l.String())
}

func BenchmarkFrameString(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = Caller(0).String()
	}
}

func BenchmarkFrameCaller(b *testing.B) {
	b.ReportAllocs()

	var l Frame

	for i := 0; i < b.N; i++ {
		l = Caller(0)
	}

	_ = l
}

func BenchmarkFrameNameFileLine(b *testing.B) {
	b.ReportAllocs()

	var n, f string
	var line int

	l := Caller(0)

	for i := 0; i < b.N; i++ {
		n, f, line = l.nameFileLine()
	}

	_, _, _ = n, f, line //nolint:dogsled
}
