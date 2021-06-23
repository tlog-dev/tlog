package tlog

import (
	"testing"

	"github.com/nikandfor/loc"
	"github.com/nikandfor/tlog/low"
	"github.com/nikandfor/tlog/wire"
	"github.com/stretchr/testify/assert"
)

func TestConsoleLocations(t *testing.T) {
	var buf, raw low.Buf

	w := NewConsoleWriter(&buf, 0)
	l := New(NewTeeWriter(&raw, w))

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

	l.Event("caller", c)
	assert.Equal(t, "caller=location.go:24\n", string(buf))

	buf = buf[:0]

	l.Event("callers", cc)
	assert.Equal(t, "callers=[location.go:71, console_test.go:21]\n", string(buf))

	t.Logf("dump:\n%v", wire.Dump(raw))

}
