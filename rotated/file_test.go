package rotated

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"testing"

	"github.com/nikandfor/assert"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/low"
	"github.com/nikandfor/tlog/tlwire"
)

func TestRotation(t *testing.T) {
	var f1, f2 low.Buf

	f := Create("name.tlog.ez")
	f.OpenFile = func(n string, ff int, m os.FileMode) (w io.Writer, _ error) {
		if f1 == nil {
			w = &f1
		} else {
			w = &f2
		}

		return w, nil
		// return compress.NewEncoder(w, 1<<16), nil
	}

	l := tlog.New(f)

	l.SetLabels("a", "b", "c", nil)

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

	if len(f1) < 20 || len(f2) < 20 {
		t.Errorf("one of files is too small")
	}
}

func dumpFile(t *testing.T, f low.Buf, name string) {
	//	r := compress.NewDecoderBytes(f)
	var r io.Reader = bytes.NewReader(f)

	d, err := ioutil.ReadAll(r)
	assert.NoError(t, err)

	t.Logf("file %q\n%s", name, tlwire.Dump(d))
}

//go:noinline
func msg(tr tlog.Span, a int) {
	tr.Printw("message", "a", a)
}
