package tlog

import (
	"fmt"
	"io"
	"strings"
	"time"
)

var TimeFormat = "01-02_15:04:05.000000"

type ConsoleWriter struct {
	w          io.Writer
	timeFormat string
}

func NewConsoleWriter(w io.Writer) ConsoleWriter {
	return ConsoleWriter{
		w:          w,
		timeFormat: TimeFormat,
	}
}

func (w ConsoleWriter) Span(s *Span) {}

func (w ConsoleWriter) Log(l *Log) {
	t := time.Unix(0, int64(l.Start))
	pref := fmt.Sprintf("%s %15v:%-4d ", t.Format(w.timeFormat), l.Location.FileBase(), l.Location.Line())
	el := "\n"
	if strings.HasSuffix(l.Fmt, el) {
		el = ""
	}
	fmt.Fprintf(w.w, pref+l.Fmt+el, l.Args...)
}
