package tlog

import (
	"testing"

	"github.com/nikandfor/assert"
)

func TestVerbosity(t *testing.T) {
	assert.True(t, (&filter{f: "topic"}).matchPattern("path/to/pkg.Func", "path/to/pkg/file.go", "topic"))
	assert.True(t, (&filter{f: "topic,topic_a"}).matchPattern("path/to/pkg.Func", "path/to/pkg/file.go", "topic"))
	assert.True(t, (&filter{f: "topic+topic_a"}).matchPattern("path/to/pkg.Func", "path/to/pkg/file.go", "topic"))

	assert.True(t, (&filter{f: "=topic"}).matchPattern("path/to/pkg.Func", "path/to/pkg/file.go", "topic"))

	assert.True(t, (&filter{f: "pkg=topic"}).matchPattern("path/to/pkg.Func", "path/to/pkg/file.go", "topic"))
	assert.True(t, (&filter{f: "pkg=*"}).matchPattern("path/to/pkg.Func", "path/to/pkg/file.go", "topic"))

	assert.True(t, (&filter{f: "Func=topic"}).matchPattern("path/to/pkg.Func", "path/to/pkg/file.go", "topic"))

	assert.True(t, (&filter{f: "Type=topic"}).matchPattern("path/to/pkg.Type.Func", "path/to/pkg/file.go", "topic"))
	assert.True(t, (&filter{f: "Type=topic"}).matchPattern("path/to/pkg.(*Type).Func", "path/to/pkg/file.go", "topic"))
	assert.True(t, (&filter{f: "pkg.Type.Func=topic"}).matchPattern("path/to/pkg.(*Type).Func", "path/to/pkg/file.go", "topic"))
	assert.True(t, (&filter{f: "pkg.(*Type).Func=topic"}).matchPattern("path/to/pkg.(*Type).Func", "path/to/pkg/file.go", "topic"))
	assert.True(t, (&filter{f: "pkg.(Type).Func=topic"}).matchPattern("path/to/pkg.(*Type).Func", "path/to/pkg/file.go", "topic"))

	assert.True(t, (&filter{f: "file.go=topic"}).matchPattern("path/to/pkg.Func", "path/to/pkg/file.go", "topic"))
	assert.True(t, (&filter{f: "pkg/file.go=topic"}).matchPattern("path/to/pkg.Func", "path/to/pkg/file.go", "topic"))

	assert.False(t, (&filter{f: "topic_a,topic_b"}).matchPattern("path/to/pkg.Func", "path/to/pkg/file.go", "topic"))
	assert.False(t, (&filter{f: "pkg"}).matchPattern("path/to/pkg.Func", "path/to/pkg/file.go", "topic"))
	assert.False(t, (&filter{f: "file.go"}).matchPattern("path/to/pkg.Func", "path/to/pkg/file.go", "topic"))
	assert.False(t, (&filter{f: "Func"}).matchPattern("path/to/pkg.Func", "path/to/pkg/file.go", "topic"))
}
