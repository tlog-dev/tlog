package tlog

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTestLogger(t *testing.T) {
	var buf bytes.Buffer

	tl := NewTestLogger(t, "", &buf)
	now = func() time.Time {
		return time.Unix(9, 0)
	}

	tl.Printf("message")

	assert.Equal(t, "1970/01/01_03:00:09.000000  test_logger_test.:19  message\n", buf.String())

	tl = NewTestLogger(t, "", nil)

	tl.Printf("it must appear in test out")
}
