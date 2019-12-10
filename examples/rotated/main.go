package main

import (
	"log"
	"os"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/rotated"
)

func main() {
	f := rotated.Create("logfile_template_#.log") // # will be substituted by time of file creation
	defer f.Close()

	f.MaxSize = 1 << 30    // 1GiB
	f.Fallback = os.Stderr // in case of failure to write to file, last chance to save log message

	tlog.DefaultLogger = tlog.New(tlog.NewConsoleWriter(f, tlog.LstdFlags))

	tlog.Printf("now use it much like %v", "log.Logger")

	log.SetOutput(f) // also works for any logger or what ever needs io.Writer

	log.Printf("also appears in the log")
}
