package main

import (
	"flag"
	"os"

	"github.com/nikandfor/json"
	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/examples/sub"
)

var (
	f   = flag.Int("f", 1, "int flag")
	str = flag.String("str", "two", "string flag")
)

var ll tlog.Logger

func initComplexLogger() func() {
	w := json.NewStreamWriterBuffer(os.Stderr, make([]byte, 0x10000))

	jw := tlog.NewJSONWriter(w)

	cw := tlog.NewConsoleWriter(os.Stderr, tlog.LdetFlags|tlog.Lspans)

	tw := tlog.NewTeeWriter(cw, jw)

	ll = tlog.NewLogger(tw)

	tlog.DefaultLogger = ll // for sub

	return func() {
		w.Flush()
	}
}

func main() {
	flag.Parse()

	cl := initComplexLogger()
	defer cl()

	tlog.DumpLabelsWithDefaults(ll, "_hostname", "_pid", "myown=label", "myflag")
	ll.Printf("os.Args: %v", os.Args)

	ll.Printf("main: %d", *f)

	tr := ll.Start()
	defer tr.Finish()

	tr.Printf("main: %v", *str)

	func1(tr.ID)

	sub.Func1(0, 5)
}

func func1(id tlog.ID) {
	tr := ll.Spawn(id)
	defer tr.Finish()

	ll.Printf("func1: %d", 3)

	tr.Printf("func1: %v", "four")

	tr.Flags |= tlog.FlagError
}
