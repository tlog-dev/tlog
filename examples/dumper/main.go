package main

import (
	"os"

	"github.com/nikandfor/cli/flag"
	"github.com/nikandfor/tlog"
)

var (
	dumps  = flag.String("dump", "", "write big dumps into the file")
	dumpsf = flag.String("dump-filter", "*", "filter which cases to write")
)

// go run main.go --dump dumps.txt --dump-filter traced

func main() {
	flag.Parse()

	if n := *dumps; n != "" {
		f, err := os.Create(n)
		if err != nil {
			tlog.Fatalf("dumps: %v", err)
		}
		defer f.Close()

		tlog.DefaultLogger.AppendWriter(tlog.NewNamedDumper("dumps", "", tlog.NewConsoleWriter(f, 0)))

		tlog.SetNamedFilter("dumps", *dumpsf)
	}

	tlog.Printf("usual log message (appears in console only)")

	tlog.V("dump,pkg").PrintRaw(0, []byte("some big file content (appears in dumps file only, if selected by filter)"))

	tr := tlog.Start()

	tr.Printf("traced message")

	tr.V("dump,traced").PrintRaw(0, []byte("from traces works the same (appears in dumps file only, if selected by filter)"))

	tr.Finish()
}
