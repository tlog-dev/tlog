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

	ll.RegisterMetric("fully_qualified_metric_name_with_units", tlog.MGauge, "help message for metric that describes it", tlog.Labels{"const=labels"})

	ll.Printf("main: %d", *f)

	work()
}

func work() {
	tr := ll.Start()
	defer tr.Finish()

	tr.Printf("main: %v", *str)

	var a A
	a.func1(tr.ID)

	measuresSomething(tr)
	measuresSomething(tr) // to show that metrics are compacted from the second time
}

func measuresSomething(tr tlog.Span) {
	tr.Observe("fully_qualified_metric_name_with_units", 123.456, tlog.Labels{"algo=fast"})
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
