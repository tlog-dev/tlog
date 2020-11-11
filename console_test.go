package tlog

import (
	"encoding/hex"
	"io"
	"testing"
	"time"
	_ "unsafe"

	"github.com/stretchr/testify/assert"

	"github.com/nikandfor/tlog/low"
	"github.com/nikandfor/tlog/tlwriter"
)

func TestConsole(t *testing.T) {
	tm := time.Date(2020, time.November, 3, 21, 06, 20, 0, time.Local)

	unixnow = func() int64 { tm = tm.Add(time.Second); return tm.UnixNano() }
	now = func() time.Time { tm = tm.Add(time.Second); return tm }

	var b, r low.Buf

	eq := func(e string) {
		t.Helper()

		if !assert.Equal(t, e+"\n", string(b)) {
			t.Logf("dump:\n%v", hex.Dump(r))
		}

		r = r[:0]
		b = b[:0]
	}

	w := tlwriter.NewConsole(&b, tlwriter.LdetFlags)
	l := New(io.MultiWriter(&r, w))

	l.Printf("message: %v %v", "args", 3)

	eq(`2020-11-03_21:06:21.000000  INF  .:                    message: args 3`)

	l.Printw("attributes", "key", "value", "key2", 42)

	eq(`2020-11-03_21:06:22.000000  INF  .:                    attributes                              key=value  key2=42`)

	tr := l.Start("span_name")

	eq(`2020-11-03_21:06:23.000000  INF  .:                    span_name                               T=s`)

	tr.Finish()

	eq(`1970-01-01_03:00:00.000000  INF  .:                    T=f  elapsed_ms=1000.00   `)
}
