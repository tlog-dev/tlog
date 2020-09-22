package tlog

import "context"

type (
	ctxidkey   struct{}
	ctxspankey struct{}
)

// ContextWithID creates new context with Span ID context.Value.
// It returns the same context if id is zero.
func ContextWithID(ctx context.Context, id ID) context.Context {
	if id == (ID{}) {
		return ctx
	}
	return context.WithValue(ctx, ctxidkey{}, id)
}

// ContextWithRandomID creates new context with random Span ID context.Value.
// May be useful to enable logging in submudules even if parent trace is not started.
func ContextWithRandomID(ctx context.Context) context.Context {
	if DefaultLogger == nil {
		return ctx
	}

	DefaultLogger.mu.Lock()
	id := DefaultLogger.randID()
	DefaultLogger.mu.Unlock()

	return context.WithValue(ctx, ctxidkey{}, id)
}

// ContextWithIDOrRandom creates new context with Span ID context.Value.
// If id is zero new random ID is generated.
func ContextWithIDOrRandom(ctx context.Context, id ID) context.Context {
	if id == (ID{}) {
		return ContextWithRandomID(ctx)
	}

	return context.WithValue(ctx, ctxidkey{}, id)
}

// ContextWithSpan creates new context with Span ID context.Value.
// It returns the same context if id is zero.
func ContextWithSpan(ctx context.Context, s Span) context.Context {
	if s.ID == (ID{}) {
		return ctx
	}
	return context.WithValue(ctx, ctxspankey{}, s)
}

// IDFromContext receives Span ID from Context.
// It returns zero if no ID found.
func IDFromContext(ctx context.Context) ID {
	v := ctx.Value(ctxidkey{})
	if id, ok := v.(ID); ok {
		return id
	}
	v = ctx.Value(ctxspankey{})
	if s, ok := v.(Span); ok {
		return s.ID
	}
	return ID{}
}

// SpanFromContext loads saved by ContextWithSpan Span from Context.
// It returns empty (no-op) Span if no ID found.
func SpanFromContext(ctx context.Context) (s Span) {
	if DefaultLogger == nil {
		return Span{}
	}

	v := ctx.Value(ctxspankey{})
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
	if id == (ID{}) {
		return Span{}
	}

	return newspan(DefaultLogger, 0, id)
}

// SpawnFromContextOrStart loads saved by ContextWithSpan Span from Context.
// It starts new trace if no ID found.
func SpawnFromContextOrStart(ctx context.Context) Span {
	if DefaultLogger == nil {
		return Span{}
	}

	id := IDFromContext(ctx)

	return newspan(DefaultLogger, 0, id)
}
