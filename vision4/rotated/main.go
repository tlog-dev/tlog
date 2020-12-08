// +build ignore

package main

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"syscall"
	"time"

	"github.com/nikandfor/cli"
	"github.com/pkg/errors"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/rotated"
)

func main() {
	cli.App = cli.Command{
		Name: "rotated test cli",
		Flags: []*cli.Flag{
			cli.NewFlag("log", "stderr", "logs location"),
		},
		Commands: []*cli.Command{{
			Name:   "writer,wr,w",
			Action: writer,
			Flags: []*cli.Flag{
				cli.NewFlag("size,s", 0, "max size"),
			},
		}, {
			Name:   "reader,rd,r",
			Action: reader,
			Flags: []*cli.Flag{
				cli.NewFlag("follow,f", false, "wait for new writes to the file"),
			},
		}},
	}

	cli.RunAndExit(os.Args)
}

func writer(c *cli.Command) (err error) {
	tlog.Printf("pid: %v", os.Getpid())

	err = ioutil.WriteFile(".pid", []byte(fmt.Sprintf("%d", os.Getpid())), 0644)
	if err != nil {
		return errors.Wrap(err, "write pid")
	}

	var w io.Writer
	if n := c.String("log"); n == "-" {
		w = os.Stdout
	} else {
		f, err := rotated.NewWriter(n, 0, 0)
		if err != nil {
			return errors.Wrap(err, "open log file")
		}

		if s := c.Int("size"); s != 0 {
			tlog.Printf("max size: %v", s)
			f.MaxSize = s
		}

		f.RotateOnSignal(syscall.SIGUSR1, syscall.SIGHUP)
		f.RotateOnFileMoved()

		w = f
	}

	tlog.DefaultLogger = tlog.New(tlog.NewConsoleWriter(w, 0))

	for i := 0; ; i++ {
		ms := rand.Int63n(30)
		d := time.Duration(ms) * time.Second / 10
		time.Sleep(d)

		tlog.Printw("log", tlog.Attrs{{Name: "i", Value: i}, {Name: "pause", Value: d.String()}}...)
	}

	return nil
}

func reader(c *cli.Command) (err error) {
	rotated.SetTestLogger(tlog.DefaultLogger)

	var r io.Reader
	if n := c.String("log"); n == "-" {
		r = os.Stdin
	} else {
		f, err := rotated.NewReader(n)
		if err != nil {
			return errors.Wrap(err, "open log file")
		}

		f.Follow = c.Bool("follow")

		r = f
	}

	s := bufio.NewScanner(r)
	for s.Scan() {
		b := s.Bytes()

		fmt.Printf("%s\n", b)
	}

	if err = s.Err(); err != nil {
		return errors.Wrap(err, "scanner")
	}

	return nil
}
