package main

import (
	"context"
	"flag"

	"tlog.app/go/tlog"
	"tlog.app/go/tlog/examples/simplest/sub"
)

var (
	f   = flag.Int("f", 1, "int flag")
	str = flag.String("str", "two", "string flag")
)

func main() {
	flag.Parse()

	tlog.Printf("main: %d %q", *f, *str)

	work(context.Background())
}

func work(ctx context.Context) {
	tr := tlog.Start("work_name")
	defer tr.Finish()

	ctx = tlog.ContextWithSpan(ctx, tr)

	sub.Func(ctx, 9)
}
