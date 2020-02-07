package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/examples/simplest/sub"
)

var (
	f   = flag.Int("f", 1, "int flag")
	str = flag.String("str", "two", "string flag")
)

var ll *tlog.Logger

func initComplexLogger() func() {
	var buf bytes.Buffer // imagine it is a log file

	jw := tlog.NewJSONWriter(&buf)

	cw := tlog.NewConsoleWriter(os.Stderr, tlog.LdetFlags|tlog.Lfuncname|tlog.Lspans|tlog.Lmessagespan)
	cw.IDWidth = 20

	ll = tlog.New(cw, jw)

	tlog.DefaultLogger = ll // needed for sub package. It uses package interface (tlog.Printf)

	return func() {
		fmt.Fprintf(os.Stderr, "%s", buf.Bytes())
	}
}

func main() {
	flag.Parse()

	cl := initComplexLogger()
	defer cl()

	lab := tlog.FillLabelsWithDefaults("_hostname", "_pid", "myown=label", "myflag")
	ll.Labels(lab)
	ll.Printf("os.Args: %v", os.Args)

	ll.Printf("main: %d", *f)

	tr := ll.Start()
	defer tr.Finish()

	tr.Printf("main: %v", *str)

	var a A
	a.func1(tr.ID)

	sub.Func1(tr.ID, 5)
}

type A struct{}

func (*A) func1(id tlog.ID) {
	tr := ll.Spawn(id)
	defer tr.Finish()

	ll.Printf("func1: %d", 3)

	func() {
		tr.Printf("func1.1: %v", "four")
	}()
}
