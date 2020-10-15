package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"

	"github.com/nikandfor/tlog"
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
	cw.IDWidth = 10
	cw.LevelWidth = 3

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

	lab := tlog.FillLabelsWithDefaults("_hostname", "_pid", "myown=label", "label_from_flags")
	ll.SetLabels(lab)
	ll.Printf("os.Args: %v", os.Args)

	ll.RegisterMetric("fully_qualified_metric_name_with_units", tlog.MGauge, "help message for metric that describes it")

	ll.Printf("main: %d", *f)

	work()
}

func work() {
	tr := ll.Start()
	defer tr.Finish()

	tr.Printw("work", tlog.AStr("argument", *str))

	var a A
	a.func1(tr.ID)

	tr.SetLabels(tlog.Labels{"a", "b=c"})

	tr.Observe("fully_qualified_metric_name_with_units", 123.456, nil)
}

type A struct{}

func (*A) func1(id tlog.ID) {
	tr := ll.Spawn(id)
	defer tr.Finish()

	tr.PrintRaw(0, tlog.WarnLevel, "func1: %v", tlog.Args{"error message"}, tlog.Attrs{{Name: "attribute", Value: "value"}})

	func() {
		tr.Errorf("func1.1: %v", 3)
	}()
}
