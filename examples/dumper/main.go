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
// Dumping requests and responses could be helpful in that situation but writing them to the same file as logs
// makes logs hard to read. Cnditional dumper can help.
//
// Initialize separate Logger with separate file as destination, set filter to it and inspect reqeust and responses content when needed.
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

		dumper = tlog.New(tlog.NewConsoleWriter(f, 0))
		dumper.SetFilter(*dumpf)
	}

	tlog.Printf("usual log message (appears in console only)")

	dumper.V("dump,pkg").PrintBytes(0, []byte("some big file content (appears in dumps file only, if selected by filter)"))

	request()
}

var dumper *tlog.Logger

func request() {
	// Should be Spawned from client trace
	tr := tlog.Start()
	defer tr.Finish()

	tr.Printf("request information")

	// Add your own description for dump (third option to identify dump).
	dumper.V("dump,traced").Migrate(tr).Printf("dump description\n%s", []byte("from traces works the same (appears in dumps file only, if selected by filter)"))
}

// Example outout:
//
// $ go run ./examples/dumper/
// 2020/08/20_05:18:33  ________________  usual log message (appears in console only)
// 2020/08/20_05:18:33  295cb45b66b75747  request information
//
// $ go run ./examples/dumper/ --dump dumper.log
// 2020/08/20_05:18:37  ________________  usual log message (appears in console only)
// 2020/08/20_05:18:37  644a9505e11726c8  request information
// $ cat dumper.log
// some big file content (appears in dumps file only, if selected by filter)
// dump description (spanid 644a9505e11726c8)
// from traces works the same (appears in dumps file only, if selected by filter)
