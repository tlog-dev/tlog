//+build !linux
//+build !darwin

package main

import (
	"github.com/nikandfor/cli"
	"github.com/nikandfor/errors"

	"github.com/nikandfor/tlog/parse"
)

var NotSupported = errors.New("not supported on that platform")

func dbdump(c *cli.Command) error {
	return NotSupported
}

func renderFromDB(c *cli.Command) (err error) {
	return NotSupported
}

func openWriter(c *cli.Command, n string) (w parse.Writer, cl func() error, err error) {
	return openWriterNoDB(c, n)
}
