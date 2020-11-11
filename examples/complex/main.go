package main

import (
	"flag"
	"os"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/tlt"
	"github.com/nikandfor/tlog/tlwriter"
	"github.com/nikandfor/tlog/wire"
)

var (
	f   = flag.Int("f", 1, "int flag")
	str = flag.String("str", "two", "string flag")
)

var ll *tlog.Logger

func initComplexLogger() func() {
	//	var buf bytes.Buffer // imagine it is a log file

	//	jw := tlog.NewJSONWriter(&buf)

	cw := tlwriter.NewConsole(os.Stderr, tlwriter.LdetFlags|tlwriter.Lfuncname|tlwriter.Lspans|tlwriter.Lmessagespan)
	cw.Shortfile = 14
	cw.Funcname = 14
	cw.IDWidth = 10
	cw.LevelWidth = 3

	ll = tlog.New(cw)

	tlog.DefaultLogger = ll // needed for sub package. It uses package interface (tlog.Printf)

	return func() {
		//	fmt.Fprintf(os.Stderr, "%s", buf.Bytes())
	}
}

func main() {
	flag.Parse()

	cl := initComplexLogger()
	defer cl()

	lab := tlt.FillLabelsWithDefaults("_hostname", "_pid", "myown=label", "label_from_flags")
	ll.SetLabels(lab)
	ll.Printf("os.Args: %v", os.Args)

	ll.RegisterMetric("fully_qualified_metric_name_with_units", tlog.Gauge, "help message for metric that describes it")

	ll.Printf("main: %d", *f)

	work()
}

func work() {
	tr := ll.Start()
	defer tr.Finish()

	tr.Printw("work", "argument", *str)

	var a A
	a.func1(tr.ID)

	//	tr.SetLabels(tlog.Labels{"a", "b=c"})

	tr.Observe("fully_qualified_metric_name_with_units", 123.456)
}

type A struct{}

func (*A) func1(id tlog.ID) {
	tr := ll.Spawn(id)
	defer tr.Finish()

	tr.Event([]wire.Tag{
		{T: wire.Level, V: tlog.Warn},
		{T: wire.Message, V: wire.Format{
			Fmt:  "func1: %v",
			Args: []interface{}{"error message"},
		}},
	}, []interface{}{"attribute", "value"})

	func() {
		tr.Event([]wire.Tag{
			{T: wire.Level, V: tlog.Error},
			{T: wire.Message, V: wire.Format{
				Fmt:  "func1.1: %v",
				Args: []interface{}{3},
			}},
		}, nil)
	}()
}
