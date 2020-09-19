// run it as go run ./examples/extend/main.go
// +build gorun

package main

import (
	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/examples/extend"
)

func main() {
	l := tlog.New(tlog.NewConsoleWriter(tlog.Stderr, tlog.LstdFlags))

	w := extend.LoggerWith(l, nil)
	w.Printw("message", extend.Fields{"args": "as_key_value"})

	f(w)
}

func f(w extend.Wrapper) {
	wctx := w.With(extend.Fields{"global": "param"}) // consider using Span.SetLabels

	wctx.Printw("made some action", extend.Fields{"local": "param"})
}
