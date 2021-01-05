package main

import (
	"fmt"
	"io"
	"os"

	"github.com/nikandfor/cli"
	"github.com/nikandfor/errors"
	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/compress"
	"github.com/nikandfor/tlog/convert"
	"github.com/nikandfor/tlog/ext/tlflag"
	"github.com/nikandfor/tlog/rotated"
)

type (
	ReadCloser struct {
		io.Reader
		io.Closer
	}

	filereader struct {
		n string
		f *os.File
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
			Name:   "convert,conv,cat,c",
			Action: conv,
			Args:   cli.Args{},
			Flags: []*cli.Flag{
				cli.NewFlag("output,out,o", "-", "output file (empty is stderr, - is stdout)"),
			},
		}, {
			Name:        "tlz",
			Description: "logs compressor/decompressor",
			Flags: []*cli.Flag{
				cli.NewFlag("output,o", "-", "output file (or stdout)"),
			},
			Commands: []*cli.Command{{
				Name:   "compress,c",
				Action: tlz,
				Args:   cli.Args{},
				Flags: []*cli.Flag{
					cli.NewFlag("block,b", 1*rotated.MB, "compression block size"),
				},
			}, {
				Name:   "decompress,d",
				Action: tlz,
				Args:   cli.Args{},
			}},
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

	/*
		if q := c.String("clickhouse"); q != "" {
			db, err := tldb.OpenClickhouse(q)
			if err != nil {
				return errors.Wrap(err, "clickhouse")
			}

			w = tlog.NewTeeWriter(w, db)
		}
	*/

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

func tlz(c *cli.Command) (err error) {
	var rs []io.Reader
	for _, a := range c.Args {
		if a == "-" {
			rs = append(rs, os.Stdin)
		} else {
			rs = append(rs, &filereader{n: a})
		}
	}

	if len(rs) == 0 {
		rs = append(rs, os.Stdin)
	}

	var w io.Writer
	if q := c.String("output"); q == "" || q == "-" {
		w = os.Stdout
	} else {
		f, err := os.Create(q)
		if err != nil {
			return errors.Wrap(err, "open output")
		}
		defer func() {
			e := f.Close()
			if err == nil {
				err = e
			}
		}()

		w = f
	}

	if c.MainName() == "compress" {
		e := compress.NewEncoder(w, c.Int("block"))

		for _, r := range rs {
			_, err = io.Copy(e, r)
			if err != nil {
				return errors.Wrap(err, "copy")
			}
		}
	} else {
		d := compress.NewDecoder(io.MultiReader(rs...))

		_, err = io.Copy(w, d)
		if err != nil {
			return errors.Wrap(err, "copy")
		}
	}

	return nil
}

func (f *filereader) Read(p []byte) (n int, err error) {
	if f.f == nil {
		f.f, err = os.Open(f.n)
		if err != nil {
			return 0, errors.Wrap(err, "open %v", f.n)
		}
	}

	n, err = f.f.Read(p)

	if err != nil {
		_ = f.f.Close()
	}

	return
}
