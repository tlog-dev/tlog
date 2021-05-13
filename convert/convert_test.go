package convert

import (
	"bytes"
	"testing"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/low"
	"github.com/nikandfor/tlog/wire"
	"github.com/stretchr/testify/assert"
)

func TestCopy(t *testing.T) {
	var src low.Buf

	l := tlog.New(&src)

	l.SetLabels(tlog.Labels{"a=b", "c"})

	l.Printw("message", "arg", "value", "int", 4)

	//	tr := l.Start("span_name")
	//	tr.Printw("traced", "message", "arg")
	//	tr.Finish("err", nil)

	//	tr.Observe("metric", 123)

	t.Logf("src\n%s", wire.Dump(src))
	//	println("src\n" + wire.Dump(src))

	var dst low.Buf

	err := Copy(NewJSONWriter(&dst), bytes.NewReader(src))
	assert.NoError(t, err)

	t.Logf("data\n%s", dst)

	if t.Failed() {
		var dump low.Buf
		err := Copy(wire.NewDumper(&dump), bytes.NewReader(src))
		if err != nil {
			t.Logf("dump (%v)\n%s", err, wire.Dump(src))
		} else {
			t.Logf("dump\n%s", dump)
		}
	}
}
