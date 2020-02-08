package main

import (
	"os"

	"github.com/nikandfor/cli/flag"
	"github.com/nikandfor/tlog"
)

var (
	dump  = flag.String("dump", "", "write big dumps into the file")
	dumpf = flag.String("dump-filter", "*", "filter which cases to write")
)

// run as
// go run main.go --dump dumps.txt --dump-filter traced

// Imagine you work with big requests and responses in some rpc or database.
// Sometimes you got unexpected behaviour, you logged add the important details, but situation is still unclear.
// Dumping requests and responses could be helpfull in that situation but writing them to the same file as logs
// makes logs hard to read. Cnditional dumper can help.
//
// Add NamedDumper with separate file as destination set filter to it and inspect reqeust and responses content when needed.

func main() {
	flag.Parse()

	// Enable printing Span ID for every message to match messages with dump (optional)
	tlog.DefaultLogger = tlog.New(tlog.NewConsoleWriter(os.Stderr, tlog.LstdFlags|tlog.Lmessagespan))

	// Enable dumps when needed
	if n := *dump; n != "" {
		// Separate file
		f, err := os.Create(n)
		if err != nil {
			tlog.Fatalf("dumps: %v", err)
		}
		defer f.Close()

		tlog.DefaultLogger.AppendWriter(
			tlog.NewNamedDumper( // Skip unconditionals, tlog.NewNamedFilter will capture all unconditional messages either.
				"dumps",                      // Name to change filter later. And to distinguish from default writer (it has name "").
				*dumpf,                       // Initial filter value.
				tlog.NewConsoleWriter(f, 0))) // Any flags could be used here if you want. tlog.Lmessagespan could help match log messages with dumps

	}

	tlog.Printf("usual log message (appears in console only)")

	tlog.V("dump,pkg").Write([]byte("some big file content (appears in dumps file only, if selected by filter)"))

	request()
}

func request() {
	// Should be Spawned from client trace
	tr := tlog.Start()
	defer tr.Finish()

	tr.Printf("request information")

	// Add your own description for dump (third option to identify dump).
	tr.V("dump,traced").Printf("dump description\n%s", []byte("from traces works the same (appears in dumps file only, if selected by filter)"))
}

// Example outout:
//
// $ go run ./examples/dumper/
// 2020/02/08_15:59:13  ________________  usual log message (appears in console only)
// 2020/02/08_15:59:13  e9540718d06cceab  request information
//
// $ go run ./examples/dumper/ --dump dumps.log
// 2020/02/08_16:16:04  ________________  usual log message (appears in console only)
// 2020/02/08_16:16:04  c927194bd7694d86  request information
// $ cat dumps.log
// some big file content (appears in dumps file only, if selected by filter)
// dump description
// from traces works the same (appears in dumps file only, if selected by filter)
