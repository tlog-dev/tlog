package tlog

import (
	"io"
	"log"
	"testing"
)

func NewTestWriter(t testing.TB) io.Writer {
	return newTestingWriter(t)
}

func NewTestLogger(t testing.TB, v string, tostderr bool) *Logger {
	var w io.Writer = log.Writer()
	ff := LdetFlags

	if t != nil && !tostderr {
		w = newTestingWriter(t)
		ff = 0
	}

	tl := New(NewConsoleWriter(w, ff))

	if v != "" {
		tl.SetFilter(v)
	}

	return tl
}
