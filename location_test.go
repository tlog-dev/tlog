package tlog

import (
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLocation(t *testing.T) {
	testLocationInside(t)
}

func testLocationInside(t *testing.T) {
	pc := Caller(0)
	name, file, line := pc.NameFileLine()
	assert.Equal(t, "tlog.testLocationInside", path.Base(name))
	assert.Equal(t, "location_test.go", path.Base(file))
	assert.Equal(t, 15, line)
}

func TestLocationShort(t *testing.T) {
	pc := Caller(0)
	assert.Equal(t, "location_test.go:23", pc.String())
}

func TestLocation2(t *testing.T) {
	func() {
		func() {
			l := Funcentry(0)

			assert.Equal(t, "location_test.go:29", l.String())
		}()
	}()
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
