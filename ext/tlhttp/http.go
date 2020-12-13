package tlhttp

import (
	"net/http"

	"github.com/nikandfor/tlog"
)

var XTraceIDKey = "X-Traceid"

func SpawnOrStart(l *tlog.Logger, req *http.Request, kvs ...interface{}) tlog.Span {
	var trid tlog.ID
	var err error

	xtr := req.Header.Get(XTraceIDKey)
	if xtr != "" {
		trid, err = tlog.IDFromString(xtr)
		if err != nil {
			trid = tlog.ID{}
		}
	}

	tr := l.NewSpan(1, trid, "http_request", kvs...)

	if err != nil {
		tr.Printf("bad parent trace id %v: %v", xtr, err)
	}

	return tr
}
