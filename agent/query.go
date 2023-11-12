package agent

import (
	"context"
	"io"
)

type (
	sub struct {
		io.Writer

		id int64
	}
)

func (a *Agent) Query(ctx context.Context, w io.Writer, ts int64, q string) error {
	return nil
}

func (a *Agent) Subscribe(ctx context.Context, w io.Writer, q string) (int64, error) {
	defer a.mu.Unlock()
	a.mu.Lock()

	a.subid++
	id := a.subid

	sub := sub{
		Writer: w,
		id:     id,
	}

	a.subs = append(a.subs, sub)

	return id, nil
}

func (a *Agent) Unsubscribe(ctx context.Context, id int64) error {
	defer a.mu.Unlock()
	a.mu.Lock()

	i := 0

	for i < len(a.subs) && a.subs[i].id < id {
		i++
	}

	if i == len(a.subs) || a.subs[i].id != id {
		return ErrUnknownSubscription
	}

	copy(a.subs[i:], a.subs[i+1:])

	a.subs = a.subs[:len(a.subs)-1]

	return nil
}
