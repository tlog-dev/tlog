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
	id := GetFullID(ctx)
	return DefaultLogger.Spawn(id)
}
