package main

import (
	"os"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/rotated"
)

func main() {
	f := rotated.Create("logfile_template_@.log") // ? will be substituted by time of file creation
	defer f.Close()

	f.Mode = 0660
	f.MaxSize = 1 << 30    // 1GiB
	f.Fallback = os.Stderr // in case of failure to write to the file, last chance to save log message

	tlog.DefaultLogger = tlog.New(tlog.NewConsoleWriter(f, tlog.LdetFlags))

	tlog.Printf("now log files will not exceed 1GiB")
}
