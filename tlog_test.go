package tlog

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLogf(t *testing.T) {
	defer func(f func() time.Time) {
		now = f
	}(now)
	now = func() time.Time {
		t, _ := time.Parse("2006-01-02_15:04:05.000000", "2019-05-09_17:43:00.122044")
		return t
	}

	var buf bytes.Buffer
	Root.w = NewConsoleWriter(&buf)

	Logf("simple message with args: %v %v %v", "str", 33, map[string]string{"a": "b"})

	assert.EqualValues(t, "05-09_20:43:00.122044    tlog_test.go:23   simple message with args: str 33 map[a:b]\n", buf.String())
}
