package main

import (
	"debug/elf"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"github.com/fsnotify/fsnotify"
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
				cli.NewFlag("output,out,o", "-:dm", "output file (empty is stderr, - is stdout)"),
				cli.NewFlag("follow,f", false, "wait for changes until terminated"),
			},
		}, {
			Name:        "seen,tlz",
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
		}, {
			Name:   "agent",
			Action: agent,
			Args:   cli.Args{},
		}, {
			Name:   "ticker",
			Action: ticker,
			Flags: []*cli.Flag{
				cli.NewFlag("output,o", "-", "output file (or stdout)"),
				cli.NewFlag("interval,int,i", time.Second, "interval to tick on"),
			},
		}, {
			Name:        "core",
			Description: "core dump memory dumper",
			Args:        cli.Args{},
			Action:      coredump,
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

func agent(c *cli.Command) error {
	if c.Args.Len() == 0 {
		return errors.New("arguments expected")
	}

	f := c.Args.First()

	r, err := tlflag.OpenReader(f)
	if err != nil {
		return errors.Wrap(err, "open: %v", f)
	}

	d := tlog.NewDecoder(r)
	i := 0
	cnt := 0

	for {
		end := d.Skip(i)
		if errors.Is(d.Err(), io.EOF) {
			tlog.Printw("EOF. wait...")
			time.Sleep(500 * time.Millisecond)

			d.ResetErr()

			continue
		}
		if err = d.Err(); err != nil {
			return errors.Wrap(err, "reading event")
		}

		cnt++

		tlog.Printw("read event", "events", cnt, "st", i, "end", end)

		i = end

		if false && cnt%3 == 0 {
			tlog.Printw("truncate file", "events", cnt, "st", i)
		}
	}
}

func ticker(c *cli.Command) error {
	w, err := tlflag.OpenWriter(c.String("output"))
	if err != nil {
		return errors.Wrap(err, "open output")
	}

	l := tlog.New(w)

	t := time.NewTicker(c.Duration("interval"))
	defer t.Stop()

	for t := range t.C {
		l.Printw("current time", "time", t)
	}

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

	var fs *fsnotify.Watcher

	if c.Bool("follow") {
		fs, err = fsnotify.NewWatcher()
		if err != nil {
			return errors.Wrap(err, "create fs watcher")
		}

		defer func() {
			e := fs.Close()
			if err == nil {
				err = errors.Wrap(e, "close watcher")
			}
		}()
	}

	rs := make(map[string]io.ReadCloser, c.Args.Len())
	defer func() {
		for name, r := range rs {
			if r != nil {
				e := r.Close()
				if err == nil {
					err = errors.Wrap(e, "close: %v", name)
				}
			}
		}
	}()

	for _, a := range c.Args {
		fs.Add(a)

		rs[a], err = tlflag.OpenReader(a)
		if err != nil {
			return errors.Wrap(err, "open: %v", a)
		}

		err = convert.Copy(w, rs[a])
		if err != nil {
			return errors.Wrap(err, "copy: %v", a)
		}
	}

	if !c.Bool("follow") {
		return nil
	}

	sigc := make(chan os.Signal, 3)
	signal.Notify(sigc, os.Interrupt)

	var ev fsnotify.Event
	for {
		select {
		case ev = <-fs.Events:
		case <-sigc:
			return nil
		case err = <-fs.Errors:
			return errors.Wrap(err, "watch")
		}

		//	tlog.Printw("fs event", "name", ev.Name, "op", ev.Op)

		if ev.Op&fsnotify.Write != 0 {
			rc, ok := rs[ev.Name]
			if !ok {
				return errors.New("unexpected event: %v", ev.Name)
			}

			err = convert.Copy(w, rc)
			if err != nil {
				return errors.Wrap(err, "copy: %v", ev.Name)
			}
		}
	}
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

func coredump(c *cli.Command) (err error) {
	if c.Args.Len() != 1 {
		return errors.New("one arg expected")
	}

	f, err := elf.Open(c.Args.First())
	if err != nil {
		return errors.Wrap(err, "open")
	}

	sym, err := f.Symbols()
	if err != nil {
		return errors.Wrap(err, "get symbols")
	}

	for _, sym := range sym {
		tlog.Printw("symbol", "sym", sym)
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
