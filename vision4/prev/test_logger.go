package tlog

import (
	"io"
	"testing"
)

// NewTestLogger creates new logger with Writer destunation of testing.T (like t.Logf).
// v is verbosity topics.
// if tostderr is not nil than destination is changed to tostderr. Useful in case if test crashed and all log output is lost.
func NewTestLogger(t testing.TB, v string, tostderr io.Writer) *Logger {
	w := tostderr
	ff := LdetFlags

	if t != nil && w == nil {
		w = newTestingWriter(t)
		ff = 0
	}

	tl := New(NewConsoleWriter(w, ff))

	if v != "" {
		tl.SetFilter(v)
	}

	return tl
}
