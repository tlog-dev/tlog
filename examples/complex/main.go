package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/convert"
	"github.com/nikandfor/tlog/low"
	"github.com/nikandfor/tlog/tlio"
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

	var jsonBuf low.Buf
	jw := convert.NewJSONWriter(&jsonBuf)

	w := tlio.NewTeeWriter(
		cw,
		jw,
	//	wire.NewDumper(tlog.Stderr),
	)

	logger = tlog.New(w)

	tlog.DefaultLogger = logger // needed for sub package. It uses package interface (tlog.Printf)

	return func() {
		fmt.Fprintf(os.Stderr, "%s", jsonBuf)
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

	logger.Printf("os.Args: %q", os.Args)

	logger.RegisterMetric("fully_qualified_metric_name_with_units", tlog.MetricGauge, "help message for metric that describes it")

	logger.Printw("main", "flag", *f)

	work()
}

func work() {
	tr := logger.Start("worker")
	defer tr.Finish()

	tr.Printw("work", "argument", *str, "hex", tlog.Hex(16))

	var a A
	a.func1(tlog.ContextWithSpan(context.Background(), tr))

	tr.Observe("fully_qualified_metric_name_with_units", 123.456)
}

type A struct{}

func (*A) func1(ctx context.Context) {
	tr := tlog.SpawnFromContext(ctx, "subtask")
	defer tr.Finish()

	tr.Printw("some warning",
		"user_attribute", "attr_value",
		tlog.KeyLogLevel, tlog.Warn)
}
