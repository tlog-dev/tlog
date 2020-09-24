package main

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/nikandfor/cli"
	"github.com/nikandfor/errors"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/parse"
)

type (
	nopCloser struct {
		io.Writer
	}
)

func main() {
	cli.App = cli.Command{
		Name:        "tlog",
		Description: "tracelog cmd",
		Before:      before,
		Flags: []*cli.Flag{
			cli.NewFlag("verbosity,v", "", "verbosity"),
			cli.HelpFlag,
			cli.FlagfileFlag,
		},
		Commands: []*cli.Command{{
			Name:   "convert,c",
			Action: convert,
			Usage:  "{infile} <outfile>",
		}, {
			Name:   "render",
			Action: render,
		}, {
			Name:        "db",
			Action:      dbdump,
			Usage:       "<db>",
			Description: "dumps db",
		}},
	}

	cli.RunAndExit(os.Args)
}

func before(c *cli.Command) error {
	tlog.SetFilter(c.String("v"))

	return nil
}

func render(c *cli.Command) (err error) {
	return renderFromDB(c)
}

func convert(c *cli.Command) (err error) {
	if c.Args.Len() < 2 {
		return errors.New("arguments expected")
	}

	w, clw, err := openWriter(c, c.Args[c.Args.Len()-1])
	if err != nil {
		return errors.Wrap(err, "open output")
	}
	defer func() {
		if e := clw(); err == nil {
			err = e
		}
	}()

	var r parse.LowReader
	for _, a := range c.Args[:c.Args.Len()-1] {
		//	tlog.Printf("convert %v...", a)

		var cl func() error
		r, cl, err = openReader(c, a)
		if err != nil {
			err = errors.Wrap(err, "open input")
			return
		}

		err = parse.Copy(w, r)
		if e := cl(); err == nil {
			err = e
		}
		if errors.Is(err, io.EOF) {
			continue
		}

		if err != nil {
			err = errors.Wrap(err, "%v", a)
			return
		}
	}

	return nil
}

//nolint:goconst
func openWriterNoDB(c *cli.Command, n string) (w parse.Writer, cl func() error, err error) {
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

func openReader(c *cli.Command, n string) (r parse.LowReader, cl func() error, err error) {
	ext := filepath.Ext(n)
	ext = strings.TrimPrefix(ext, ".")

	var fr io.ReadCloser

	switch ext {
	case "json",
		"protobuf", "proto", "pb":
		fr, err = fropen(c, n)

		cl = fr.Close
	default:
		err = errors.New("undefined reader format: %v", ext)
		return
	}

	switch ext {
	case "json":
		r = parse.NewJSONReader(fr)
	case "protobuf", "proto", "pb":
		r = parse.NewProtoReader(fr)
	}

	return
}

func fropen(c *cli.Command, n string) (io.ReadCloser, error) {
	ext := filepath.Ext(n)
	if n := strings.TrimSuffix(n, ext); n == "-" {
		return ioutil.NopCloser(os.Stdin), nil
	}

	ff := os.O_RDONLY

	f, err := os.OpenFile(n, ff, 0)
	if err != nil {
		return nil, err
	}

	return f, nil
}

func fwopen(c *cli.Command, n string) (io.WriteCloser, error) {
	ext := filepath.Ext(n)
	if n := strings.TrimSuffix(n, ext); n == "-" {
		return nopCloser{os.Stdout}, nil
	}

	ff := os.O_RDWR | os.O_CREATE
	if !c.Bool("force") {
		ff |= os.O_EXCL
	}

	f, err := os.OpenFile(n, ff, 0644)
	if err != nil {
		return nil, err
	}

	return f, nil
}

func (c nopCloser) Close() error { return nil }
