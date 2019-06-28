package main

import (
	"os"

	"github.com/nikandfor/json"
	"github.com/nikandfor/tlog"
)

var ll tlog.Logger

func initComplexLogger() func() {
	w := json.NewStreamWriterBuffer(os.Stderr, make([]byte, 0x10000))

	jw := tlog.NewJSONWriter(w)

	cw := tlog.NewConsoleWriter(os.Stderr)

	tw := tlog.NewTeeWriter(cw, jw)

	ll = tlog.NewLogger(tw)

	return func() {
		w.Flush()
	}
}

func main() {
	cl := initComplexLogger()
	defer cl()

	ll.Printf("main: %d", 1)

	tr := ll.Start()
	defer tr.Finish()

	tr.Printf("main: %v", "two")

	func1(tr.ID)
}

func func1(id tlog.FullID) {
	tr := ll.Spawn(id)
	defer tr.Finish()

	ll.Printf("func1: %d", 3)

	tr.Printf("func1: %v", "four")

	tr.Flags |= tlog.FlagError
}
