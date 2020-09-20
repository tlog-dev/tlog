package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/nikandfor/cli"
	"github.com/nikandfor/errors"
	"github.com/nikandfor/xrain"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/parse"
	"github.com/nikandfor/tlog/tlogdb"
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
	// parent span
	// root span
	// child spans
	// spans by messages
	// messages by query
	// logical equation with queries and labels

	b, err := xrain.Mmap(c.Args.First(), os.O_CREATE|os.O_RDONLY)
	if err != nil {
		return err
	}
	defer b.Close()

	db, err := xrain.NewDB(b, 0, nil)
	if err != nil {
		return err
	}

	err = db.View(func(tx *xrain.Tx) error {
		for _, a := range c.Args {
			id := tlog.ShouldID(tlog.IDFromString(a))

			bi := tx.Bucket([]byte("i"))
			t := bi.Tree()

			st, _ := t.Seek(id[:], nil, nil)
			k, _ := t.Key(st, nil)

			if !bytes.HasPrefix(id[:], k) {
				fmt.Printf("Span %x not found\n", id)

				continue
			}

			ts := t.Value(st, nil)

			bs := tx.Bucket([]byte("s"))
			sval := bs.Get(ts)

			fmt.Printf("Span %s\n", sval)
		}

		return nil
	})

	return
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

func dbdump(c *cli.Command) error {
	if c.Args.Len() == 0 {
		return errors.New("argument expected")
	}

	b, err := xrain.Mmap(c.Args.First(), os.O_CREATE|os.O_RDONLY)
	if err != nil {
		return err
	}
	defer b.Close()

	db, err := xrain.NewDB(b, 0, nil)
	if err != nil {
		return err
	}

	err = db.View(func(tx *xrain.Tx) error {
		xrain.DebugDump(os.Stdout, tx.SimpleBucket)

		return nil
	})

	return err
}

func openReader(c *cli.Command, n string) (r parse.LowReader, cl func() error, err error) {
	ext := filepath.Ext(n)
	ext = strings.TrimPrefix(ext, ".")

	var fr io.ReadCloser

	switch ext {
	case "json":
		fallthrough
	case "protobuf", "proto", "pb":
		fr, err = fropen(c, n)

		cl = func() error {
			return fr.Close()
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

func openWriter(c *cli.Command, n string) (w parse.Writer, cl func() error, err error) {
	ext := filepath.Ext(n)
	ext = strings.TrimPrefix(ext, ".")

	var fw io.WriteCloser
	var dbb *xrain.MmapBack

	switch ext {
	case "json":
		fallthrough
	case "protobuf", "proto", "pb":
		fallthrough
	case "log", "":
		fw, err = fwopen(c, n)
		if err != nil {
			return
		}

		cl = func() error {
			return fw.Close()
		}
	case "tldb", "tlogdb", "db":
		dbb, err = xrain.Mmap(n, os.O_CREATE|os.O_RDWR)
		if err != nil {
			return
		}

		cl = func() error {
			return dbb.Close()
		}
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
	case "tldb", "tlogdb", "db":
		var xdb *xrain.DB
		xdb, err = xrain.NewDB(dbb, 0, nil)
		if err != nil {
			return
		}

		db := tlogdb.NewDB(xdb)

		w, err = tlogdb.NewWriter(db)
		if err != nil {
			return
		}
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
