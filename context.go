package tlog

import "context"

type key struct{}

func ContextWithID(ctx context.Context, id ID) context.Context {
	if id == 0 {
		return ctx
	}
	return context.WithValue(ctx, key{}, id)
}

func IDFromContext(ctx context.Context) ID {
	v := ctx.Value(key{})
	id, _ := v.(ID)
	return id
}

func SpawnFromContext(ctx context.Context) Span {
	if DefaultLogger == nil {
		return Span{}
	}

	id := IDFromContext(ctx)
	if id == 0 {
		return Span{}
	}

	return newspan(DefaultLogger, id)
}
