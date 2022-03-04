package main

import (
	"os"

	"github.com/nikandfor/cli"
	"github.com/nikandfor/tlog/cmd/tlog/tlogmain"
)

func main() {
	app := tlogmain.App()

	cli.RunAndExit(app, os.Args, os.Environ())
}
