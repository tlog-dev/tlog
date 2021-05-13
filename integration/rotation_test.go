package integration

import (
	"io"
	"io/ioutil"
	"os"
	"testing"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/compress"
	"github.com/nikandfor/tlog/low"
	"github.com/nikandfor/tlog/rotated"
	"github.com/nikandfor/tlog/wire"
	"github.com/stretchr/testify/assert"
)

func TestRotation(t *testing.T) {
	var f1, f2 low.Buf

	f := rotated.Create("name.tlog.seen")
	f.OpenFile = func(n string, ff int, m os.FileMode) (io.Writer, error) {
		if f1 == nil {
			return &f1, nil
		}

		return &f2, nil
	}

	c := compress.NewEncoder(f, 1<<16)

	l := tlog.New(c)

	l.SetLabels(tlog.Labels{"a=b", "c"})

	tr := l.Start("some_span")

	msg(tr, 1)
	msg(tr, 2)

	err := f.Rotate()
	if err != nil {
		assert.NoError(t, err)
	}

	msg(tr, 3)

	dumpFile(t, f1, "first")
	dumpFile(t, f2, "second")
}

func dumpFile(t *testing.T, f low.Buf, name string) {
	r := compress.NewDecoderBytes(f)

	d, err := ioutil.ReadAll(r)
	assert.NoError(t, err)

	t.Logf("file %q\n%s", name, wire.Dump(d))
}

//go:noinline
func msg(tr tlog.Span, a int) {
	tr.Printw("message", "a", a)
}
