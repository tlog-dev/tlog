package tlog

import "context"

type key struct{}

func WithFullID(ctx context.Context, id FullID) context.Context {
	return context.WithValue(ctx, key{}, id)
}

func GetFullID(ctx context.Context) FullID {
	v := ctx.Value(key{})
	id, _ := v.(FullID)
	return id
}

func SpawnFromContext(ctx context.Context) *Span {
	if DefaultLogger == nil {
		return nil
	}

	id := GetFullID(ctx)
	if id.TraceID == 0 {
		return nil
	}

	s := &Span{
		l:        DefaultLogger,
		ID:       FullID{id.TraceID, SpanID(rnd.Int63())},
		Parent:   id.SpanID,
		Location: funcentry(1),
		Start:    now(),
	}
	DefaultLogger.SpanStarted(s)
	return s
}
