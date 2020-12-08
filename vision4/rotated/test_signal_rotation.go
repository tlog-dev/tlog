// +build ignore

package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"sync/atomic"
	"syscall"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/rotated"
)

var (
	file = flag.String("file,f", "logfile", "file to write logs to")
)

func main() {
	f := rotated.CreateLogrotate(*file)
	defer func() {
		err := f.Close()
		if err != nil {
			tlog.Printf("close: %v", err)
		}
	}()
	f.Mode = 0660

	tlog.Printf("writing infinite logs to file %v", *file)

	var i int64

	go func() {
		l := tlog.New(tlog.NewConsoleWriter(f, tlog.LdetFlags))

		for {
			v := atomic.AddInt64(&i, 1)

			l.Printf("line: %v", v)
		}
	}()

	r := bufio.NewReader(os.Stdin)

	j := 0
	for {
		tlog.Printf("press enter to rotate file")

		_, _ = r.ReadString('\n')

		j++
		m := fmt.Sprintf("%v.%v", *file, j)

		v := atomic.AddInt64(&i, 1)

		tlog.Printf("mv at step %d : %v to %v", v, *file, m)

		err := os.Rename(*file, m)
		if err != nil {
			tlog.Fatalf("mv: %v", err)
		}

		err = syscall.Kill(os.Getpid(), syscall.SIGUSR1)
		if err != nil {
			tlog.Fatalf("kill: %v", err)
		}
	}
}
