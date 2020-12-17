package main

import (
	"fmt"
	"io"
	"os"

	"github.com/nikandfor/cli"
	"github.com/nikandfor/errors"
	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/convert"
	"github.com/nikandfor/tlog/ext/tlflag"
)

type (
	ReadCloser struct {
		io.Reader
		io.Closer
	}
)

func main() {
	cli.App = cli.Command{
		Name:   "tlog",
		Before: before,
		Flags: []*cli.Flag{
			cli.NewFlag("log", "stderr", "log output file (or stderr)"),
			cli.NewFlag("verbosity,v", "", "logger verbosity topics"),
			cli.FlagfileFlag,
			cli.HelpFlag,
		},
		Commands: []*cli.Command{{
			Name:   "convert",
			Action: conv,
			Args:   cli.Args{},
			Flags: []*cli.Flag{
				cli.NewFlag("output,out,o", "-", "output file (empty is stderr, - is stdout)"),
			},
		}, {
			Name:        "seen",
			Description: "logs compressor/decompressor",
			Action:      seen,
		}},
	}

	err := cli.Run(os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%+v\n", err)
		os.Exit(1)
	}
}

func before(c *cli.Command) error {
	w, err := tlflag.OpenWriter(c.String("log"))
	if err != nil {
		return errors.Wrap(err, "open log file")
	}

	tlog.DefaultLogger = tlog.New(w)

	tlog.SetFilter(c.String("verbosity"))

	return nil
}

func conv(c *cli.Command) error {
	w, err := tlflag.OpenWriter(c.String("out"))
	if err != nil {
		return err
	}
	defer func() {
		e := w.Close()
		if err == nil {
			err = e
		}
	}()

	//	tlog.Printf("writer: %T %[1]v", w)

	for _, a := range c.Args {
		err = func() (err error) {
			r, err := tlflag.OpenReader(a)
			if err != nil {
				return errors.Wrap(err, a)
			}
			defer func() {
				e := r.Close()
				if err == nil {
					err = e
				}
			}()

			//	tlog.Printf("reader: %T %[1]v", r)

			err = convert.Copy(w, r)
			if err != nil {
				return errors.Wrap(err, "copy")
			}

			return nil
		}()
		if err != nil {
			return err
		}
	}

	//	tlog.Printf("copied")

	return nil
}

func seen(c *cli.Command) error {
	return nil
}
