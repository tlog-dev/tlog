package main

import (
	"bytes"
	"fmt"
	"os"

	"github.com/nikandfor/tlog"
)

func main() {
	var buf bytes.Buffer

	tlog.DefaultLogger = tlog.New(
		tlog.NewConsoleWriter(os.Stderr, tlog.LdetFlags), // equal to tlog.NewNamedWriter("", "", tlog.NewConsoleWriter(os.Stderr, tlog.LdetFlags))
		tlog.NewNamedWriter("verbose", "topic", tlog.NewConsoleWriter(&buf, tlog.LdetFlags)))

	tlog.SetFilter("")                            // first writer. Default filter name is empty
	tlog.SetNamedFilter("verbose", "topic,debug") // second writer with defined name
	tlog.SetNamedFilter("unexisted", "subtopic")  // will silently have no effect since there is no writer with name unexisted

	fmt.Fprintf(os.Stderr, "FIRST writer (stderr):\n")

	tlog.Printf("unconditional log message")

	tlog.V("topic").Printf("simple condition")

	tlog.V("trace").Printf("simple condition (will not be printed)")

	if l := tlog.V("topic"); l != nil {
		p := 1 + 3 // make complex calculations here
		l.Printf("then log the result: %v", p)
		tlog.Printf("package interface will print to all writers not only to filtered by V")
	}

	funcUnconditionalTrace()

	fmt.Printf("SECOND writer (buf):\n%s", buf.Bytes())
}

func funcUnconditionalTrace() {
	tr := tlog.Start()
	defer tr.Finish()

	tr.Printf("traced message")

	funcConditionalTrace(tr.ID)
}

func funcConditionalTrace(id tlog.ID) {
	tr := tlog.V("debug").Spawn(id)
	defer tr.Finish()

	tr.Printf("printed only to second writer")

	if tr.Valid() {
		p := 1 + 5 // complex calculations
		tr.Printf("verbose output: %v", p)
	}

	if tr := tr.V("subtopic"); tr.Valid() { // tr redefined for convinience, counld be any name
		p := 2 + 3                              // even more complex calculations or big output
		tr.Printf("very verbose output: %v", p) // will not be printed
	}
}
