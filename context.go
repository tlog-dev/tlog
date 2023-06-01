package tlog

import (
	"context"
)

type (
	ctxspankey struct{}
)

// ContextWithSpan creates new context with Span ID context.Value.
// It returns the same context if id is zero.
func ContextWithSpan(ctx context.Context, s Span) context.Context {
	if s.Logger == nil && SpanFromContext(ctx) == (Span{}) {
		return ctx
	}

	return context.WithValue(ctx, ctxspankey{}, s)
}

// SpanFromContext loads saved by ContextWithSpan Span from Context.
// It returns valid empty (no-op) Span if none was found.
func SpanFromContext(ctx context.Context) (s Span) {
	v := ctx.Value(ctxspankey{})
	s, _ = v.(Span)

	return
}

// SpawnFromContext spawns new Span derived form Span or ID from Context.
// It returns empty (no-op) Span if no ID found.
func SpawnFromContext(ctx context.Context, name string, kvs ...interface{}) Span {
	s, ok := ctx.Value(ctxspankey{}).(Span)
	if !ok {
		return Span{}
	}

	return newspan(s.Logger, s.ID, 0, name, kvs)
}

func SpawnFromContextOrStart(ctx context.Context, name string, kvs ...interface{}) Span {
	v := ctx.Value(ctxspankey{})
	s, ok := v.(Span)
	if ok {
		return newspan(s.Logger, s.ID, 0, name, kvs)
	}

	return newspan(DefaultLogger, ID{}, 0, name, kvs)
}

func SpawnFromContextAndWrap(ctx context.Context, name string, kvs ...interface{}) (Span, context.Context) {
	s, ok := ctx.Value(ctxspankey{}).(Span)
	if !ok {
		return Span{}, ctx
	}

	s = newspan(s.Logger, s.ID, 0, name, kvs)
	ctx = context.WithValue(ctx, ctxspankey{}, s)

	return s, ctx
}
