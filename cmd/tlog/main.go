package main

import (
	"os"

	"github.com/nikandfor/cli"

	"github.com/nikandfor/tlog/cmd/tlog/tlogcmd"
)

func main() {
	app := tlogcmd.App()

	cli.RunAndExit(app, os.Args, os.Environ())
}
