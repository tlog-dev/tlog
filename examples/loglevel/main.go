package main

import "github.com/nikandfor/tlog"

func main() {
	tlog.DefaultLogger = tlog.New(tlog.NewConsoleWriter(tlog.Stderr, tlog.LdetFlags))

	tlog.Printw("info", "", tlog.Info) // empty key is auto detected by value type
	tlog.Printw("warning", "", tlog.Warn)
	tlog.Printw("error", "", tlog.Error)
	tlog.Printw("fatal", "", tlog.Fatal)
	tlog.Printw("debug", "", tlog.Debug)
	tlog.Printw("debug_3", "", tlog.LogLevel(-3))
	tlog.Printw("level_6", "", tlog.LogLevel(6))

	tlog.Printw("not a log level", tlog.KeyLogLevel, 2)
}
