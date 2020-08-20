package main

import (
	"os"
	"runtime"

	"github.com/nikandfor/tlog"
)

func main() {
	tlog.DefaultLogger = tlog.New(tlog.NewConsoleWriter(os.Stderr, tlog.LstdFlags))

	tlog.SetFilter("topic,info") // change filter in flight

	tlog.Printf("unconditional log message")

	tlog.V("topic").Printf("simple condition")

	tlog.V("debug").Printf("simple condition (will not be printed)")

	if l := tlog.V("info"); l != nil {
		dumpSomeStats(l)
	}

	funcUnconditionalTrace()
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

	tr.Printf("this line printed if filter includes topic")

	if tr.Valid() { // true if topic enabled
		p := 1 + 1 // complex calculations
		tr.Printf("verbose output: %v", p)
	}

	if tr.If("subtopic") { // true if both: topic and subtopic enabled by filter
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
