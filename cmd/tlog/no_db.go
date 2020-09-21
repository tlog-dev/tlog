//+build !linux
//+build !darwin

package main

import (
	"io"
	"path/filepath"
	"strings"

	"github.com/nikandfor/cli"
	"github.com/nikandfor/errors"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/parse"
)

var NotSupported = errors.New("not supported on that platform")

func dbdump(c *cli.Command) error {
	return NotSupported
}

func renderFromDB(c *cli.Command) (err error) {
	return NotSupported
}

//nolint:goconst
func openWriter(c *cli.Command, n string) (w parse.Writer, cl func() error, err error) {
	ext := filepath.Ext(n)
	ext = strings.TrimPrefix(ext, ".")

	var fw io.WriteCloser

	switch ext {
	case "json",
		"protobuf", "proto", "pb",
		"log", "":
		fw, err = fwopen(c, n)
		if err != nil {
			return
		}

		cl = fw.Close
	case "tldb", "tlogdb", "db":
		err = NotSupported
		return
	default:
		err = errors.New("undefined writer format: %v", ext)
		return
	}

	switch ext {
	case "json", "j":
		w = parse.NewAnyWiter(tlog.NewJSONWriter(fw))
	case "protobuf", "proto", "pb":
		w = parse.NewAnyWiter(tlog.NewProtoWriter(fw))
	case "console", "stderr", "log", "":
		w = parse.NewConsoleWriter(fw, tlog.LdetFlags|tlog.Lspans|tlog.Lmessagespan)
	}

	return
}
