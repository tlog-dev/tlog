package tlog

import "context"

type (
	key     struct{}
	spankey struct{}
)

// ContextWithID creates new context with Span ID context.Value.
// It returns the same context if id is zero.
func ContextWithID(ctx context.Context, id ID) context.Context {
	if id == z {
		return ctx
	}
	return context.WithValue(ctx, key{}, id)
}

// ContextWithRandomID creates new context with random Span ID context.Value.
// May be useful to enable logging in submudules even if parent trace is not started.
func ContextWithRandomID(ctx context.Context) context.Context {
	mu.Lock()
	id := randID()
	mu.Unlock()
	return context.WithValue(ctx, key{}, id)
}

// ContextWithIDOrRandom creates new context with Span ID context.Value.
// If id is zero new random ID is generated.
func ContextWithIDOrRandom(ctx context.Context, id ID) context.Context {
	if id == z {
		mu.Lock()
		id = randID()
		mu.Unlock()
	}
	return context.WithValue(ctx, key{}, id)
}

// ContextWithSpan creates new context with Span ID context.Value.
// It returns the same context if id is zero.
func ContextWithSpan(ctx context.Context, s Span) context.Context {
	if s.ID == z {
		return ctx
	}
	return context.WithValue(ctx, spankey{}, s)
}

// IDFromContext receives Span ID from Context.
// It returns zero if no ID found.
func IDFromContext(ctx context.Context) ID {
	v := ctx.Value(key{})
	if id, ok := v.(ID); ok {
		return id
	}
	v = ctx.Value(spankey{})
	if s, ok := v.(Span); ok {
		return s.ID
	}
	return z
}

// SpanFromContext loads saved by ContextWithSpan Span from Context.
// It returns empty (no-op) Span if no ID found.
func SpanFromContext(ctx context.Context) (s Span) {
	if DefaultLogger == nil {
		return Span{}
	}

	v := ctx.Value(spankey{})
	s, _ = v.(Span)

	return
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

// SpanFromContextOrStart loads saved by ContextWithSpan Span from Context.
// It starts new trace if no ID found.
func SpawnFromContextOrStart(ctx context.Context) Span {
	if DefaultLogger == nil {
		return Span{}
	}

	id := IDFromContext(ctx)

	return newspan(DefaultLogger, id)
}
