package agent

import (
	"context"
	"errors"
	"io"

	"github.com/nikandfor/tlog"
)

func (a *Agent) Query(ctx context.Context, w io.Writer, q string) (err error) {
	tr := tlog.SpawnFromContextOrStart(ctx, "agent_query", "query", q)
	defer tr.Finish("err", &err)

	ctx = tlog.ContextWithSpan(ctx, tr)

	id := a.registerSub(w)
	defer a.unregisterSub(id)

	<-ctx.Done()
	err = ctx.Err()

	if errors.Is(err, context.Canceled) {
		err = nil
	}

	return
}

func (a *Agent) registerSub(w io.Writer) int64 {
	defer a.Unlock()
	a.Lock()

	a.subid++
	id := a.subid

	s := &sub{
		id:     id,
		Writer: w,
	}

	a.subs[id] = s

	return id
}

func (a *Agent) unregisterSub(id int64) {
	defer a.Unlock()
	a.Lock()

	delete(a.subs, id)
}
