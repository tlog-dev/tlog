package agent

import (
	"context"
	"io"
	"net/http"
	"sync"
	"time"

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

func New() *Agent {
	a := &Agent{}

	a.Sink.Writer = a

	a.cond.L = a.mu.RLocker()

	return a
}

func (a *Agent) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	err := a.Serve(req.Context(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *Agent) Serve(ctx context.Context, w io.Writer) (err error) {
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

func (a *Agent) Write(p []byte) (_ int, err error) {
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
