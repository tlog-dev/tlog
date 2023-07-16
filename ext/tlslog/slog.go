package tlslog

import (
	"context"
	"time"

	"github.com/nikandfor/loc"
	"golang.org/x/exp/slog"

	"tlog.app/go/tlog"
)

type (
	Handler struct {
		*tlog.Logger
		Level slog.Level

		b []byte

		prefix []byte
		depth  int
	}
)

var _ slog.Handler = &Handler{}

func Wrap(l *tlog.Logger) *Handler {
	return &Handler{Logger: l}
}

func (l *Handler) Enabled(ctx context.Context, lvl slog.Level) bool {
	return l != nil && l.Logger != nil && lvl >= l.Level
}

func (l *Handler) Handle(ctx context.Context, r slog.Record) error { //nolint:gocritic
	if l == nil {
		return nil
	}

	defer l.Unlock()
	l.Lock()

	l.b = l.AppendMap(l.b[:0], -1)

	if r.Time != (time.Time{}) {
		l.b = l.AppendString(l.b, tlog.KeyTimestamp)
		l.b = l.AppendTime(l.b, r.Time)
	}

	if r.PC != 0 {
		l.b = l.AppendKey(l.b, tlog.KeyCaller)
		l.b = l.AppendCaller(l.b, loc.PC(r.PC))
	}

	l.b = l.AppendKey(l.b, tlog.KeyMessage)
	l.b = l.AppendSemantic(l.b, tlog.WireMessage)
	l.b = l.Encoder.AppendString(l.b, r.Message)

	if r.Level != 0 {
		l.b = l.AppendKey(l.b, tlog.KeyLogLevel)
		l.b = level(r.Level).TlogAppend(l.b)
	}

	l.b = append(l.b, l.prefix...)

	r.Attrs(l.attr)

	for i := 0; i < l.depth; i++ {
		l.b = l.AppendBreak(l.b)
	}

	l.b = append(l.b, l.Logger.Labels()...)

	l.b = l.AppendBreak(l.b)

	_, err := l.Writer.Write(l.b)

	return err
}

func (l *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return l
	}

	defer l.Unlock()
	l.Lock()

	b := l.b
	l.b = append([]byte{}, l.prefix...)

	for _, a := range attrs {
		l.attr(a)
	}

	p := l.b
	l.b = b

	return &Handler{
		Logger: l.Logger,
		Level:  l.Level,
		prefix: p,
		depth:  l.depth,
	}
}

func (l *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return l
	}

	p := append([]byte{}, l.prefix...)

	p = l.AppendKey(p, name)
	p = l.AppendMap(p, -1)

	return &Handler{
		Logger: l.Logger,
		Level:  l.Level,
		prefix: p,
		depth:  l.depth + 1,
	}
}

func (l *Handler) attr(a slog.Attr) bool {
	kind := a.Value.Kind()

	if a.Key == "" && kind != slog.KindGroup {
		return true
	}

	val := a.Value.Resolve()

	if kind != slog.KindGroup {
		l.b = l.AppendKey(l.b, a.Key)
		l.b = l.AppendValue(l.b, val.Any())

		return true
	}

	gr := val.Group()

	if len(gr) == 0 {
		return true
	}

	if a.Key != "" {
		l.b = l.AppendKey(l.b, a.Key)
		l.b = l.AppendMap(l.b, len(gr))
	}

	for _, a := range gr {
		ok := l.attr(a)
		if !ok {
			return false
		}
	}

	return true
}

func level(lvl slog.Level) tlog.LogLevel {
	return tlog.LogLevel(lvl / 4)
}
