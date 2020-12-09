package main

import (
	"flag"
	"os"

	"github.com/nikandfor/tlog"
)

var (
	f      = flag.Int("f", 1, "int flag")
	str    = flag.String("str", "two", "string flag")
	labels = flag.String("labels", "label_from_flags", "comma separated labels")
)

var logger *tlog.Logger

func initComplexLogger() func() {
	//	var buf bytes.Buffer // imagine it is a log file

	//	jw := tlog.NewJSONWriter(&buf)

	cw := tlog.NewConsoleWriter(os.Stderr, tlog.LdetFlags|tlog.Lfuncname)
	cw.Shortfile = 14
	cw.Funcname = 14
	cw.IDWidth = 10

	logger = tlog.New(cw)

	tlog.DefaultLogger = logger // needed for sub package. It uses package interface (tlog.Printf)

	return func() {
		//	fmt.Fprintf(os.Stderr, "%s", buf.Bytes())
	}
}

func main() {
	flag.Parse()

	cl := initComplexLogger()
	defer cl()

	ll := tlog.ParseLabels(*labels)
	ll = append(tlog.Labels{"_hostname", "_pid", "myown=label"}, ll...)
	ll = tlog.FillLabelsWithDefaults(ll...)

	logger.SetLabels(ll)

	logger.Printf("os.Args: %v", os.Args)

	logger.RegisterMetric("fully_qualified_metric_name_with_units", tlog.MetricGauge, "help message for metric that describes it")

	logger.Printf("main: %d", *f)

	work()
}

func work() {
	tr := logger.Start("worker")
	defer tr.Finish()

	tr.Printw("work", "argument", *str)

	var a A
	a.func1(tr.ID)

	tr.Observe("fully_qualified_metric_name_with_units", 123.456)
}

type A struct{}

func (*A) func1(id tlog.ID) {
	tr := logger.Spawn(id, "subtask")
	defer tr.Finish()

	tr.Event(tlog.KeyLogLevel, tlog.Warn,
		tlog.KeyMessage, tlog.Format{
			Fmt:  "func1: %v",
			Args: []interface{}{"some warning"}},
		"user_attribute", "value",
	)

	func() {
		tr.Event(tlog.KeyLogLevel, tlog.Error,
			tlog.KeyMessage, tlog.Format{
				Fmt:  "func1: %v - %v",
				Args: []interface{}{"some error", 3}},
		)
	}()
}
