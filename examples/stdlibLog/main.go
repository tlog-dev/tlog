package main

import (
	"log"
	"os"

	"github.com/nikandfor/tlog"
)

func main() {
	// stdlib logger
	log.Printf("message %d", 1)

	// tlog logger
	l := tlog.New(tlog.NewConsoleWriter(os.Stderr, tlog.LdetFlags))

	l.Printf("message %d", 2)

	// use tlog under stdlib interface
	l.DepthCorrection = 2

	log.SetFlags(0)
	log.SetOutput(l)

	log.Printf("message %d", 3)

}
