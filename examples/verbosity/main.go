package main

import (
	"bytes"
	"fmt"
	"os"
	"runtime"

	"github.com/nikandfor/tlog"
)

func main() {
	var buf bytes.Buffer // Imagine this is file

	tlog.DefaultLogger = tlog.New(
		tlog.NewConsoleWriter(os.Stderr, tlog.LstdFlags), // standard logs to stderr
		tlog.NewNamedWriter( // more detailed logs to a file
			"verbose", // name to address filter later
			"*",       // capture all topics
			tlog.NewConsoleWriter(&buf, tlog.LdetFlags))) // writer with more detailed flags

	tlog.SetNamedFilter("verbose", "topic,debug") // change filter in flight

	fmt.Fprintf(os.Stderr, "STDERR OUTPUT:\n")

	tlog.Printf("unconditional log message")

	tlog.V("topic").Printf("simple condition")

	tlog.V("trace").Printf("simple condition (will not be printed)")

	if l := tlog.V("debug"); l != nil { // l is a *tlog.Logger
		dumpSomeStats(l)
	}

	funcUnconditionalTrace()

	// file content:
	fmt.Fprintf(os.Stderr, "FILE CONTENT:\n%s", buf.Bytes())
}

func funcUnconditionalTrace() {
	tr := tlog.Start()
	defer tr.Finish()

	tr.Printf("traced message")

	funcConditionalTrace(tr.ID)
}

func funcConditionalTrace(id tlog.ID) {
	tr := tlog.V("topic").Spawn(id)
	defer tr.Finish()

	tr.Printf("printed only to a file")

	if tr.Valid() { // true if topic enabled
		p := 1 + 1 // complex calculations
		tr.Printf("verbose output: %v", p)
	}

	if tr2 := tr.V("subtopic"); tr2.Valid() { // true if both: topic and subtopic enabled by filter
		p := 2 + 3 + 4 // even more complex calculations
		tr.Printf("very verbose output: %v", p)
	}
}

func dumpSomeStats(l *tlog.Logger) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// big output
	l.Printf(`mem stats:
heap:              %v,
cumulative allocs: %v
gc called:         %v
`, m.HeapAlloc, m.TotalAlloc, m.NumGC)
}
