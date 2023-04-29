package tlog

import (
	"io"
	"testing"

	"github.com/nikandfor/assert"
	"github.com/nikandfor/loc"

	"github.com/nikandfor/tlog/low"
	"github.com/nikandfor/tlog/tlwire"
)

type TeeWriter []io.Writer

func TestConsoleLocations(t *testing.T) {
	var buf, raw low.Buf

	w := NewConsoleWriter(&buf, 0)
	l := New(TeeWriter{&raw, w})

	w.PadEmptyMessage = false

	c := loc.Caller(-1)
	cc := loc.Callers(-1, 2)

	{
		name, file, line := c.NameFileLine()
		t.Logf("caller: %v %v %v", name, file, line)

		for _, pc := range cc {
			name, file, line := pc.NameFileLine()
			t.Logf("callers: %v %v %v", name, file, line)
		}
	}

	_ = l.Event("caller", c)
	assert.Equal(t, "caller=location.go:24\n", string(buf))

	buf = buf[:0]

	_ = l.Event("callers", cc)
	assert.Equal(t, "callers=[location.go:71 console_test.go:25]\n", string(buf))

	t.Logf("dump:\n%v", tlwire.Dump(raw))
}

func (w TeeWriter) Write(p []byte) (n int, err error) {
	for i, w := range w {
		m, e := w.Write(p)

		if i == 0 {
			n = m
		}

		if err == nil {
			err = e
		}
	}

	return
}
