package main

import (
	"flag"

	"github.com/nikandfor/tlog"
)

var (
	f   = flag.Int("f", 1, "int flag")
	str = flag.String("str", "two", "string flag")
)

func main() {
	flag.Parse()

	tlog.DefaultLabels.Set("mylabel", "value")
	tlog.DefaultLabels.Set("myflag", "")

	tlog.Printf("main: %d %q", *f, *str)
}
