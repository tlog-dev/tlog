package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/nikandfor/errors"
	"github.com/nikandfor/loc"
	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/convert"
	"github.com/nikandfor/tlog/tlio"
	"github.com/nikandfor/tlog/wire"
)

type (
	Agent struct {
		Sink

		db *DB

		router *httprouter.Router

		//

		mu   sync.RWMutex
		cond sync.Cond

		evs []*event

		d wire.Decoder
	}

	event struct {
		Timestamp time.Time

		Spans []tlog.ID
		Refs  []ref

		Kind tlog.EventKind

		Msg []byte

		Labels []byte

		vals []value

		raw []byte
	}

	value struct {
		name []byte
		val  []byte

		tag byte
		sub int64
	}

	ref struct {
		Name []byte
		Span tlog.ID
	}
)

func New(dbpath string) (*Agent, error) {
	a := &Agent{}

	a.Sink.Writer = a

	if dbpath != "" {
		db, err := NewDB(dbpath)
		if err != nil {
			return nil, errors.Wrap(err, "open db")
		}

		a.db = db
	}

	a.setupRoutes()

	a.cond.L = a.mu.RLocker()

	return a, nil
}

func (a *Agent) setupRoutes() {
	a.router = httprouter.New()

	a.router.HandlerFunc("GET", "/labels", a.ServeLabels)
	a.router.HandlerFunc("GET", "/streams", a.ServeStreams)
	a.router.HandlerFunc("GET", "/events", a.ServeEvents)

	a.router.HandlerFunc("GET", "/", func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprintf(w, "%slabels  - list of known labels\n", req.RequestURI)
		fmt.Fprintf(w, "%sstreams - list of streams\n", req.RequestURI)
		fmt.Fprintf(w, "%sevents  - stream of events\n", req.RequestURI)
	})
}

func (a *Agent) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	tr := tlog.SpanFromContext(req.Context())
	tr.Printw("request", "uri", req.RequestURI, "path", req.URL.Path)

	a.router.ServeHTTP(w, req)
}

func (a *Agent) ServeLabels(w http.ResponseWriter, req *http.Request) {
	if a.db == nil {
		http.Error(w, "no persistance", http.StatusServiceUnavailable)
		return
	}

	ls := a.db.allLabels()

	_ = json.NewEncoder(w).Encode(ls)
}

func (a *Agent) ServeStreams(w http.ResponseWriter, req *http.Request) {
	if a.db == nil {
		http.Error(w, "no persistance", http.StatusServiceUnavailable)
		return
	}

	ss := a.db.allStreams()

	_ = json.NewEncoder(w).Encode(ss)
}

func (a *Agent) ServeEvents(w http.ResponseWriter, req *http.Request) {
	tr := tlog.SpanFromContext(req.Context())

	q := req.URL.Query()

	var start int64
	if q := q.Get("start"); q != "" {
		var err error
		start, err = strconv.ParseInt(q, 10, 64)
		if err != nil {
			err = errors.Wrap(err, "start")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		start = time.Now().UnixNano()
	}

	var sid uint32
	if l, ok := q["stream"]; !ok || len(l) == 0 {
		http.Error(w, "stream is required", http.StatusBadRequest)
		return
	} else {
		l[0] = strings.TrimPrefix(l[0], "0x")

		x, err := strconv.ParseUint(l[0], 16, 32)
		if err != nil {
			err = errors.Wrap(err, "stream")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		sid = uint32(x)
	}

	var ww io.Writer = w

	switch q.Get("format") {
	case "text":
		ww = tlog.NewConsoleWriter(ww, tlog.LstdFlags|tlog.Lmilliseconds)
	default: // json
		ww = convert.NewJSONWriter(ww)
	}

	f, ok := w.(tlio.Flusher)
	if !ok {
		f2, ok2 := w.(tlio.FlusherNoError)
		if ok2 {
			f = tlio.WrapFlusherNoError(f2)
			ok = true
		}
	}

	tr.V("flusher").Printw("writer is flusher", "ok", ok)

	if ok && ww != w {
		ww = tlio.WriteFlusher{
			Writer:  ww,
			Flusher: f,
		}
	}

	err := a.db.Stream(req.Context(), ww, start, sid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (a *Agent) Serve(ctx context.Context, w io.Writer) (err error) {
	return nil
}

func (a *Agent) Write(p []byte) (_ int, err error) {
	if a.db == nil {
		return len(p), nil
	}

	_, err = a.db.Write(p)
	if err != nil {
		return 0, errors.Wrap(err, "db")
	}

	return len(p), nil
}

func (a *Agent) Serve0(ctx context.Context, w io.Writer) (err error) {
	defer a.mu.RUnlock()
	a.mu.RLock()

	jw := convert.NewJSONWriter(w)

	i := 0

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		for i < len(a.evs) {
			_, err = jw.Write(a.evs[i].raw)
			if err != nil {
				return errors.Wrap(err, "write")
			}

			i++
		}

		if f, ok := w.(tlio.Flusher); ok {
			err = f.Flush()
			if err != nil {
				return errors.Wrap(err, "flush")
			}
		}

		if f, ok := w.(interface{ Flush() }); ok {
			f.Flush()
		}

		a.cond.Wait()
	}
}

func (a *Agent) Write0(p []byte) (_ int, err error) {
	tlog.V("events").Printw("event", "raw", wire.Dump(p), "from", loc.Callers(1, 3))

	ev := new(event)

	ev.raw = p

	tag, els, i := a.d.Tag(p, 0)
	if tag != wire.Map {
		return 0, errors.New("map expected")
	}

	for el := 0; els == -1 || el < int(els); el++ {
		if els == -1 || a.d.Break(p, &i) {
			break
		}

		i = a.parseVal(p, i, ev)
	}

	defer a.mu.Unlock()
	a.mu.Lock()

	a.cond.Broadcast()

	a.evs = append(a.evs, ev)

	return len(p), nil
}

func (a *Agent) parseVal(p []byte, st int, ev *event) (i int) {
	k, i := a.d.String(p, st)

	st = i

	tag, sub, _ := a.d.Tag(p, i)
	if tag != wire.Semantic {
		return a.simpleVal(p, k, i, ev)
	}

	switch {
	case sub == wire.Time && string(k) == tlog.KeyTime:
		ev.Timestamp, i = a.d.Time(p, i)
	case sub == tlog.WireLabels && string(k) == tlog.KeyLabels:
		i = a.d.Skip(p, i)

		ev.Labels = p[st:i]
	case sub == tlog.WireID:
		var s tlog.ID
		i = s.TlogParse(&a.d, p, i)

		if string(k) == tlog.KeySpan {
			ev.Spans = append(ev.Spans, s)
		} else {
			ev.Refs = append(ev.Refs, ref{
				Name: k,
				Span: s,
			})
		}
	case sub == tlog.WireEventKind && string(k) == tlog.KeyEventKind:
		i = ev.Kind.TlogParse(&a.d, p, i)
	case sub == tlog.WireMessage && string(k) == tlog.KeyMessage:
		k, i = a.d.String(p, i)

		ev.Msg = k
	default:
		i = a.simpleVal(p, k, i, ev)
	}

	return
}

func (a *Agent) simpleVal(p, k []byte, st int, ev *event) (i int) {
	i = a.d.Skip(p, st)

	ev.vals = append(ev.vals, value{
		name: k,
		val:  p[st:i],
	})

	return
}
