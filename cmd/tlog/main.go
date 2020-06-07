package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nikandfor/cli"
	"github.com/nikandfor/errors"
	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/parse"
)

func main() {
	cli.App = cli.Command{
		Name:        "tlog",
		Description: "tracelog cmd",
		Before:      before,
		Flags: []*cli.Flag{
			cli.NewFlag("verbosity,v", "", "verbosity"),
		},
		Commands: []*cli.Command{{
			Name:   "convert,c",
			Action: convert,
			Flags: []*cli.Flag{
				cli.NewFlag("file,f", "", "file to read from (- for stdin)"),
				cli.NewFlag("output,o", "", "file to write to (- for stdout)"),
				cli.NewFlag("infmt,if", "", "input file format (json, protobuf)"),
				cli.NewFlag("outfmt,of", "", "output file format (json, protobuf, console)"),
			},
		}},
	}

	cli.RunAndExit(os.Args)
}

func before(c *cli.Command) error {
	tlog.SetFilter(c.String("v"))

	return nil
}

func convert(c *cli.Command) error {
	var inext, outext string

	var fr io.Reader = os.Stdin
	if q := c.String("file"); q != "" && q != "-" {
		f, err := os.Open(q)
		if err != nil {
			return errors.Wrap(err, "input file")
		}

		defer f.Close()

		fr = f

		inext = filepath.Ext(q)
		inext = strings.TrimPrefix(inext, ".")
	}

	var fw io.Writer = os.Stdout
	if q := c.String("output"); q != "" && q != "-" {
		f, err := os.Create(q)
		if err != nil {
			return errors.Wrap(err, "output file")
		}

		defer f.Close()

		fw = f

		outext = filepath.Ext(q)
		outext = strings.TrimPrefix(outext, ".")
	}

	var r parse.LowReader

	q := c.String("infmt")
	if q == "" {
		q = inext
	}

	switch q {
	case "json", "j":
		r = parse.NewJSONReader(fr)
	case "protobuf", "proto", "pb":
		r = parse.NewProtoReader(fr)
	default:
		return errors.New("undefined reader format: %v", q)
	}

	var w parse.Writer

	q = c.String("outfmt")
	if q == "" {
		q = outext
	}

	switch q {
	case "json", "j":
		w = parse.NewAnyWiter(tlog.NewJSONWriter(fw))
	case "protobuf", "proto", "pb":
		w = parse.NewAnyWiter(tlog.NewProtoWriter(fw))
	case "console", "stderr", "log", "":
		w = parse.NewConsoleWriter(fw, tlog.LdetFlags|tlog.Lspans|tlog.Lmessagespan)
	default:
		return errors.New("undefined writer format: %v", q)
	}

loop:
	for {
		tp, err := r.Type()
		if err == io.EOF {
			break
		}
		if err != nil {
			tlog.Printf("read record: %v", err)
			break
		}

		switch tp {
		case 'L':
			ls, err := r.Labels()
			if err != nil {
				tlog.Printf("read record body: %v", err)
				break loop
			}

			err = w.Labels(ls)
			if err != nil {
				tlog.Printf("write record: %v", err)
				break loop
			}
		case 'l':
			l, err := r.Location()
			if err != nil {
				tlog.Printf("read record body: %v", err)
				break loop
			}

			err = w.Location(l)
			if err != nil {
				tlog.Printf("write record: %v", err)
				break loop
			}
		case 'm':
			m, err := r.Message()
			if err != nil {
				tlog.Printf("read record body: %v", err)
				break loop
			}

			err = w.Message(m)
			if err != nil {
				tlog.Printf("write record: %v", err)
				break loop
			}
		case 's':
			s, err := r.SpanStart()
			if err != nil {
				tlog.Printf("read record body: %v", err)
				break loop
			}

			err = w.SpanStart(s)
			if err != nil {
				tlog.Printf("write record: %v", err)
				break loop
			}
		case 'f':
			f, err := r.SpanFinish()
			if err != nil {
				tlog.Printf("read record body: %v", err)
				break loop
			}

			err = w.SpanFinish(f)
			if err != nil {
				tlog.Printf("write record: %v", err)
				break loop
			}
		default:
			tlog.Printf("unexpected record type: %v", tp)

			_, err = r.Read()
			if err != nil {
				tlog.Printf("read record body: %+v", err)
				break loop
			}
		}
	}

	return nil
}
