package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/nikandfor/cli"
	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/ext/tlflag"
	"github.com/nikandfor/tlog/writer"
)

func main() {
	cli.App = cli.Command{
		Name:        "tlog",
		Description: "tracelog cmd",
		Before:      before,
		Flags: []*cli.Flag{
			cli.NewFlag("rewrite", false, "rewrite existing output file if exists"),
			cli.NewFlag("verbosity,v", "", "verbosity"),
			cli.HelpFlag,
			cli.FlagfileFlag,
		},
		Commands: []*cli.Command{{
			Name:   "convert,cat,c",
			Action: convert,
			Usage:  "{infiles} <outfile>",
			Flags: []*cli.Flag{
				cli.NewFlag("detach,d", false, "run in background"),
				cli.NewFlag("follow,f", false, "process new data as file grows"),
			},
		}},
	}

	cli.RunAndExit(os.Args)
}

func before(c *cli.Command) error {
	tlog.SetFilter(c.String("v"))

	return nil
}

func convert(c *cli.Command) (err error) {
	if c.Args.Len() == 0 {
		return errors.New("arguments expected")
	}

	var ins []string
	var out io.Writer

	if l := c.Args.Len(); l == 1 {
		ins = c.Args
		out = writer.NewConsole(os.Stderr, writer.LstdFlags)
	} else {
		out, err = tlflag.ParseDestination(c.Args[l-1])
		if err != nil {
			return errors.Wrap(err, "open writer")
		}

		ins = c.Args[:l-1]
	}

	for _, a := range ins {
		ext := filepath.Ext(a)

		var in io.Reader
		if a == "-" || a == "" {
			in = os.Stdin
		} else {
			var f io.ReadCloser

			switch ext {
			case "", ".log":
				return errors.New("reading console output is not supported")
			case ".tl", ".tlog":
			default:
				return errors.New("unsupported input file extension")
			}
		}
	}

	return nil
}
