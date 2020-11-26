package tlog

import (
	"io"
	"math/rand"
	"sync"
	"testing"
	"time"
	_ "unsafe"

	"github.com/stretchr/testify/assert"

	"github.com/nikandfor/tlog/low"
	"github.com/nikandfor/tlog/wire"
	"github.com/nikandfor/tlog/writer"
)

func testRandID() func() ID {
	var mu sync.Mutex
	rnd := rand.New(rand.NewSource(0))

	return func() (id ID) {
		mu.Lock()
		for id == (ID{}) {
			_, _ = rnd.Read(id[:])
		}
		mu.Unlock()
		return
	}
}

func TestConsole(t *testing.T) {
	tm := time.Date(2020, time.November, 3, 21, 06, 20, 0, time.Local)

	unixnow = func() int64 { tm = tm.Add(time.Second); return tm.UnixNano() }
	now = func() time.Time { tm = tm.Add(time.Second); return tm }

	var b, r low.Buf

	eq := func(e string) {
		t.Helper()

		if !assert.Equal(t, e+"\n", string(b)) {
			t.Logf("dump:\n%v", wire.Dump(r))
		}

		r = r[:0]
		b = b[:0]
	}

	w := writer.NewConsole(&b, writer.LdetFlags|writer.Lspans|writer.Lmessagespan)
	l := New(io.MultiWriter(&r, w))
	l.NewID = testRandID()

	l.Printf("message: %v %v", "args", 3)

	eq(`2020-11-03_21:06:21.000000  INF  console_test.go:55    ________  message: args 3`)

	l.Printw("attributes", "key", "value", "key2", 42)

	eq(`2020-11-03_21:06:22.000000  INF  console_test.go:59    ________  attributes                    key=value  key2=42`)

	tr := l.Start("span_name")

	eq(`2020-11-03_21:06:23.000000  INF  console_test.go:63    0194fdc2  span_name                     T=s`)

	tr.Finish()

	eq(`1970-01-01_03:00:00.000000  INF  .:                    0194fdc2  T=f  elapsed_ms=1000`)
}
