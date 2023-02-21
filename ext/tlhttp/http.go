package tlhttp

import (
	"net/http"

	"github.com/nikandfor/tlog"
)

var TraceIDKey = "Traceid"

func SpawnOrStart(l *tlog.Logger, req *http.Request, kvs ...interface{}) tlog.Span {
	var trid tlog.ID
	var err error

	xtr := req.Header.Get(TraceIDKey)
	if xtr != "" {
		trid, err = tlog.IDFromString(xtr)
	}

	tr := l.NewSpan(1, trid, "http_request", kvs...)

	if err != nil {
		tr.Printw("bad parent trace id", "id", xtr, "err", err)
	}

	return tr
}
