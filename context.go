package tlog

import "context"

type key struct{}

// ContextWithID creates new context with Span ID context.Value.
// It returns the same context if id is 0
func ContextWithID(ctx context.Context, id ID) context.Context {
	if id == z {
		return ctx
	}
	return context.WithValue(ctx, key{}, id)
}

// IDFromContext receives Span ID from Context.
// It returns zero if no ID found.
func IDFromContext(ctx context.Context) ID {
	v := ctx.Value(key{})
	id, _ := v.(ID)
	return id
}

// SpawnFromContext spawns new Span derived form Span ID from Context.
// It returns empty (no-op) Span if no ID found.
func SpawnFromContext(ctx context.Context) Span {
	if DefaultLogger == nil {
		return Span{}
	}

	id := IDFromContext(ctx)
	if id == z {
		return Span{}
	}

	return newspan(DefaultLogger, id)
}
