package tlog

import "context"

type key struct{}

func WithID(ctx context.Context, id ID) context.Context {
	return context.WithValue(ctx, key{}, id)
}

func GetID(ctx context.Context) ID {
	v := ctx.Value(key{})
	return v.(ID)
}

func SpawnFromContext(ctx context.Context) *Span {
	if DefaultLogger == nil {
		return nil
	}

	id := GetID(ctx)
	if id == 0 {
		return nil
	}

	s := &Span{
		l:        DefaultLogger,
		ID:       ID(rnd.Int63()),
		Parent:   id,
		Location: funcentry(1),
		Start:    now(),
	}
	DefaultLogger.SpanStarted(s)
	return s
}
