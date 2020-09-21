//+build linux darwin

package main

import (
	"bytes"
	"fmt"
	"io"
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

func renderFromDB(c *cli.Command) (err error) {
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

//nolint:goconst
func openWriter(c *cli.Command, n string) (w parse.Writer, cl func() error, err error) {
	ext := filepath.Ext(n)
	ext = strings.TrimPrefix(ext, ".")

	var fw io.WriteCloser
	var dbb *xrain.MmapBack

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
		dbb, err = xrain.Mmap(n, os.O_CREATE|os.O_RDWR)
		if err != nil {
			return
		}

		cl = dbb.Close
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
