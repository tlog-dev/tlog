package main

import (
	"os"

	"nikand.dev/go/cli"

	"tlog.app/go/tlog/cmd/tlog/tlogcmd"
)

func main() {
	app := tlogcmd.App()

	cli.RunAndExit(app, os.Args, os.Environ())
}
