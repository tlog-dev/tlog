package tlhttp

import (
	"net/http"

	"github.com/nikandfor/tlog"
)

var TraceIDKey = "Traceid"

func SpawnOrStart(w http.ResponseWriter, req *http.Request, kvs ...interface{}) tlog.Span {
	return spawnOrStart(tlog.DefaultLogger, w, req, kvs)
}

func SpawnOrStartLogger(l *tlog.Logger, w http.ResponseWriter, req *http.Request, kvs ...interface{}) tlog.Span {
	return spawnOrStart(l, w, req, kvs)
}

func spawnOrStart(l *tlog.Logger, w http.ResponseWriter, req *http.Request, kvs []interface{}) tlog.Span {
	var trid tlog.ID
	var err error

	xtr := req.Header.Get(TraceIDKey)
	if xtr != "" {
		trid, err = tlog.IDFromString(xtr)
	}

	tr := l.NewSpan(2, trid, "http_request", append([]interface{}{
		"client", req.RemoteAddr,
		"method", req.Method,
		"path", req.URL.Path,
	}, kvs...)...)

	if err != nil {
		tr.Printw("bad parent trace id", "id", xtr, "err", err)
	}

	w.Header().Set(TraceIDKey, tr.ID.StringFull())

	return tr
}
