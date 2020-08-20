package guess

import (
	"bytes"
	"testing"

	"github.com/nikandfor/tlog"
)

var l *Logger

func TestGuess(t *testing.T) {
	//	tl = tlog.DefaultLogger

	var buf bytes.Buffer
	l = tlog.New(tlog.NewConsoleWriter(&buf, tlog.Lspans|tlog.Lmessagespan|tlog.Lshortfile|tlog.Lfuncname))

	tr := Start(l)

	f1(1, 2)

	for i := 0; i < 3; i++ {
		f2(i)
	}

	Finish(tr)

	t.Logf("output:\n%v", buf.String())

	//	if len(c) != 0 {
	//		t.Errorf("cache: %v", c)
	//	}
}

func f1(a, b int) {
	tr := Spawn(l)
	defer Finish(tr)

	tr.Printf("f1 %v %v", a, b)

	f2(100 + a)
}

func f2(a int) {
	tr := Spawn(l)
	defer Finish(tr)

	tr.Printf("f2 %v", a)
}
