package main

import (
	"bytes"
	"log"
	"os"

	"github.com/nikandfor/tlog"
)

// If you use tlog but some of your dependancies are not, it's not a problem.

func main() {
	var buf bytes.Buffer // imagine it's file

	l := tlog.New(
		tlog.NewConsoleWriter(os.Stderr, tlog.LdetFlags), // more detailed flags
		tlog.NewJSONWriter(&buf),                         // or you want your logs in JSON
	) // or both

	l.Printf("use tlog directly")

	// pass *tlog.Logger or tlog.Span to stdlib logger
	// or anywhere io.Writer is expected

	l.DepthCorrection = 2 // correct which stack frame to record (to not record log.go:172 all the time)

	log.SetFlags(0) // hide time column produced by stdlib log
	log.SetOutput(l)

	log.Printf("use as stdlib log")
}
