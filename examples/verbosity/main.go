package main

import (
	"os"

	"github.com/nikandfor/tlog"
)

func main() {
	tlog.DefaultLogger = tlog.New(tlog.NewConsoleWriter(os.Stderr, tlog.LdetFlags))

	tlog.Printf("unconditional log message")

	tlog.V(tlog.LevError).Printf("simple condition")

	tlog.V(tlog.LevTrace).Printf("simple condition (will not be printed)")

	if l := tlog.V(tlog.LevInfo); l != nil {
		p := 1 + 3 // make complex calculations here
		l.Printf("then log the result: %v", p)
		tlog.Printf("you may use returned `l' logger or package interface")
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
	tr := tlog.V(tlog.LevDebug).Spawn(id)
	defer tr.Finish()

	tr.Printf("will not be printed because of verbosity condition of the trace")

	if tr != nil {
		p := 1 + 5 // complex calculations
		tr.Printf("this whole if will not be executed: %v", p)
	}
}
