package main

import (
	"context"
	"flag"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/examples/sub"
)

var (
	f   = flag.Int("f", 1, "int flag")
	str = flag.String("str", "two", "string flag")
)

func main() {
	flag.Parse()

	tlog.Printf("main: %d %q", *f, *str)

	sub.Func1(0, 5)

	work()
}

func work() {
	tr := tlog.Start()
	defer tr.Finish()

	ctx := tlog.WithID(context.Background(), tr.ID)

	sub.Func2(ctx, 9)
}
