package loc

import (
	"bytes"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLocation(t *testing.T) {
	testLocationInside(t)
}

func testLocationInside(t *testing.T) {
	pc := Caller(0)
	name, file, line := pc.NameFileLine()
	assert.Equal(t, "loc.testLocationInside", path.Base(name))
	assert.Equal(t, "location_test.go", filepath.Base(file))
	assert.Equal(t, 19, line)
}

func TestLocationShort(t *testing.T) {
	pc := Caller(0)
	assert.Equal(t, "location_test.go:27", pc.String())
}

func TestLocation2(t *testing.T) {
	func() {
		func() {
			l := Funcentry(0)

			assert.Equal(t, "location_test.go:33", l.String())
		}()
	}()
}

func TestLocationFormat(t *testing.T) {
	l := Caller(-1)

	var b bytes.Buffer

	fmt.Fprintf(&b, "%v", l)
	assert.Equal(t, "location.go:31", b.String())

	b.Reset()

	fmt.Fprintf(&b, "%.3v", l)
	assert.Equal(t, "location.go: 31", b.String())

	b.Reset()

	fmt.Fprintf(&b, "%18.3v", l)
	assert.Equal(t, "location.go   : 31", b.String())

	b.Reset()

	fmt.Fprintf(&b, "%+v", l)
	assert.True(t, regexp.MustCompile(`[\w./-]*location.go:31`).MatchString(b.String()))

	b.Reset()

	fmt.Fprintf(&b, "%#v", l)
	assert.Equal(t, "Caller:31", b.String())
}

func TestLocationCropFileName(t *testing.T) {
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
	l := PC(0x1234567890)

	l.SetCache("", "", 0)

	assert.NotEqual(t, "file.go:10", l.String())

	l.SetCache("Name", "file.go", 10)

	assert.Equal(t, "file.go:10", l.String())
}

func BenchmarkLocationString(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = Caller(0).String()
	}
}

func BenchmarkLocationCaller(b *testing.B) {
	b.ReportAllocs()

	var l PC

	for i := 0; i < b.N; i++ {
		l = Caller(0)
	}

	_ = l
}

func BenchmarkLocationNameFileLine(b *testing.B) {
	b.ReportAllocs()

	var n, f string
	var line int

	l := Caller(0)

	for i := 0; i < b.N; i++ {
		n, f, line = l.nameFileLine()
	}

	_, _, _ = n, f, line //nolint:dogsled
}
