package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nikandfor/cli"
	"github.com/nikandfor/errors"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/ext/tlflag"
	"github.com/nikandfor/tlog/parse"
	"github.com/nikandfor/tlog/rotated"
)

type (
	nopCloser struct {
		*os.File
	}
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
	if c.Args.Len() == 0 {
		return errors.New("arguments expected")
	}

	var w parse.Writer

	n := c.Args.Len()
	if n != 1 {
		n--
	} else {
		w = parse.NewAnyWiter(tlog.NewConsoleWriter(os.Stdout, tlog.LstdFlags))
	}

	if w == nil {
		var clw func() error

		w, clw, err = openWriter(c, c.Args[c.Args.Len()-1])
		if err != nil {
			return
		}

		defer func() {
			if e := clw(); err == nil {
				err = e
			}
		}()
	}

	var r parse.LowReader
	var cl func() error
	for _, a := range c.Args[:n] {
		tlog.V("filename").Printf("convert %v", a)

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
			err = errors.Wrap(err, "copy %v", a)
			return
		}
	}

	return nil
}

//nolint:goconst
func openWriter(c *cli.Command, n string) (w parse.Writer, cl func() error, err error) {
	var flags string
	if p := strings.LastIndexByte(n, ':'); p != -1 {
		flags = n[p+1:]
		n = n[:p]
	}

	ff := tlog.LdetFlags | tlog.Lspans | tlog.Lmessagespan
	of := os.O_RDWR | os.O_CREATE | os.O_APPEND
	ff, of = tlflag.UpdateFlags(ff, of, flags)

	ext := filepath.Ext(n)
	ext = strings.TrimPrefix(ext, ".")

	var fw io.WriteCloser

	switch ext {
	case "json",
		"protobuf", "proto", "pb",
		"log", "":
		fw, err = fwopen(c, n, of)
		if err != nil {
			return
		}

		cl = fw.Close
	case "tlogdb", "tldb", "db":
		return openDBWriter(c, n)
	default:
		err = errors.New("undefined writer format: %v", ext)
		return
	}

	switch ext {
	case "json", "j":
		w = parse.NewAnyWiter(tlog.NewJSONWriter(fw))
	case "protobuf", "proto", "pb":
		w = parse.NewAnyWiter(tlog.NewProtoWriter(fw))
	case "console", "stderr", "log", "err", "":
		w = parse.NewConsoleWriter(fw, ff)
	}

	return
}

func openReader(c *cli.Command, n string) (r parse.LowReader, cl func() error, err error) {
	ext := filepath.Ext(n)
	ext = strings.TrimPrefix(ext, ".")

	var fr io.Reader

	switch ext {
	case "json",
		"protobuf", "proto", "pb":
		fr, err = fropen(c, n)

		if cc, ok := fr.(io.Closer); ok {
			cl = cc.Close
		}
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

func fropen(c *cli.Command, n string) (io.Reader, error) {
	ext := filepath.Ext(n)
	if n := strings.TrimSuffix(n, ext); n == "-" || n == "" {
		return os.Stdin, nil
	}

	f, err := rotated.NewReader(n)
	if err != nil {
		return nil, err
	}

	f.Follow = c.Bool("follow")

	return f, nil
}

func fwopen(c *cli.Command, n string, ff int) (io.WriteCloser, error) {
	ext := filepath.Ext(n)
	if n := strings.TrimSuffix(n, ext); n == "-" || n == "" {
		return nopCloser{os.Stdout}, nil
	}

	if !c.Bool("rewrite") {
		ff |= os.O_EXCL
	}

	f, err := rotated.NewWriter(n, ff, 0644)
	if err != nil {
		return nil, err
	}

	return f, nil
}

func (nopCloser) Close() error { return nil }
