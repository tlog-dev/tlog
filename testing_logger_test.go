package tlog

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTestLogger(t *testing.T) {
	tm := time.Date(2019, time.July, 6, 19, 45, 25, 0, time.Local)

	var buf bytes.Buffer

	tl := NewTestLogger(t, "topic", &buf)

	tl.now = func() time.Time {
		return tm
	}
	tl.nano = tm.UnixNano

	tl.Printf("message")

	assert.Equal(t, tm.Format("2006-01-02_15:04:05.000000")+"  INF  testing_logger_:23  message\n", buf.String())

	tl = NewTestLogger(t, "", nil)

	t.Logf("there must be log line after that")
	tl.Printf("it must appear in test out")
}
