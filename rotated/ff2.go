// +build run

package main

import (
	"bytes"
	"fmt"
	"os"

	"github.com/nikandfor/tlog"
	"gopkg.in/fsnotify.v1"
)

func main() {
	n := "tmpfile"

	f, err := os.Create(n)
	if err != nil {
		panic(err)
	}

	defer f.Close()

	go watch(n)

	msg := make([]byte, 120)
	copy(msg, bytes.Repeat([]byte("__message__|"), 10))
	msg[len(msg)-1] = '\n'

	var s string

	for {
		_, err := f.Write(msg)
		if err != nil {
			panic(err)
		}

		fmt.Printf("press enter")
		fmt.Scanf("%s", &s)
	}
}

func watch(n string) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		panic(err)
	}

	err = w.Add(n)
	if err != nil {
		panic(err)
	}

loop:
	for {
		var ev fsnotify.Event

		select {
		case ev = <-w.Events:
		case err = <-w.Errors:
			break loop
		}

		if ev.Op == fsnotify.Write {
			continue
		}

		tlog.Printf("%v %v", ev.Op, ev.Name)
	}

	if err != nil {
		panic(err)
	}
}
