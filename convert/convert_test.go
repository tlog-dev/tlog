package convert

import (
	"bytes"
	"testing"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/low"
	"github.com/stretchr/testify/assert"
)

func TestCopy(t *testing.T) {
	var src low.Buf

	l := tlog.New(&src)

	l.SetLabels(tlog.Labels{"a=b", "c"})

	l.Printw("message", "arg", "value", "int", 4)

	tr := l.Start("span_name")
	tr.Printw("traced", "message", "arg")
	tr.Finish("err", nil)

	tr.Observe("metric", 123)

	var dst low.Buf

	err := Copy(NewJSONWriter(&dst), bytes.NewReader(src))
	assert.NoError(t, err)

	t.Logf("data\n%s", dst)

	if t.Failed() {
		t.Logf("dump\n%s", tlog.Dump(src))
	}
}
