// +build ignore

package main

import (
	"flag"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/rotated"
)

var (
	file = flag.String("file,f", "logfile", "file to write logs to")
)

func main() {
	f := rotated.Create(*file)
	defer func() {
		err := f.Close()
		if err != nil {
			tlog.Printf("close: %v", err)
		}
	}()
	f.Mode = 0660
	f.MaxSize = 1 << 20

	tlog.Printf("writing infinite logs to file %v", *file)

	l := tlog.New(tlog.NewConsoleWriter(f, tlog.LdetFlags))

	var i int64
	for {
		i++
		l.Printf("line: %v", i)
	}
}
