package processor

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/low"
	"github.com/nikandfor/tlog/tlio"
)

func TestProcessor(t *testing.T) {
	var b low.Buf
	cw := tlog.NewConsoleWriter(&b, 0)

	var count tlio.CountableIODiscard

	w := New(tlio.NewTeeWriter(cw, &count), "selected")
	w.NonTraced = false
	w.MaxDepth = 0

	l := tlog.New(w)
	l.NoCaller = true
	l.NoTime = true

	l.Printw("non-traced")

	tr := l.Start("not_selected")
	tr.Printw("traced")
	tr.Finish()

	assert.Equal(t, int64(0), count.Operations, "%s", b)

	b = b[:0]
	count.Operations = 0

	tr = l.Start("selected")

	tr2 := tr.Spawn("sub_selected")
	tr2.Printw("traced")
	tr2.Finish()

	tr.Finish()

	assert.Equal(t, int64(3), count.Operations, "%s", b)

	b = b[:0]
	count.Operations = 0

	w.MaxDepth = 1

	tr = l.Start("selected")

	tr2 = tr.Spawn("sub_selected")
	tr2.Printw("traced")

	tr3 := tr2.Spawn("sub_sub_selected")
	tr3.Finish()

	tr2.Finish()

	tr.Finish()

	assert.Equal(t, int64(6), count.Operations, "%s", b)
}
