package tlog

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConsoleWriter(t *testing.T) {
	t.Skip()

	unixnow = func() int64 { return time.Date(2020, time.November, 3, 21, 06, 21, 0, time.Local).UnixNano() }

	var b bufWriter

	w := NewConsoleWriter(&b, LdetFlags)
	l := New(w)

	l.Printf("message: %v %v", "args", 3)
	l.Printw("attributes", "key", "value", "key2", 42)

	assert.Equal(t, `2020-11-03_21:06:21.000000  INF  .:                    message: args 3
`, string(b))
}
