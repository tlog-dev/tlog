package extend

import (
	"github.com/nikandfor/tlog"
)

const MaxDepth = 4

// Although it is not recommended to dumlicate the same attributes multiple times in the same Span it is possible.
// This is example of how to extend tlog in a such way.

type (
	Attrs = tlog.Attrs

	Wrapper struct {
		*tlog.Logger
		Attrs [MaxDepth]Attrs // avoid allocations
	}

	SpanWrapper struct {
		tlog.Span
		Attrs [MaxDepth]Attrs // avoid allocations
	}

	byKey []interface{}
)

func With(fs Attrs) Wrapper {
	return Wrapper{
		Logger: tlog.DefaultLogger,
		Attrs:  [MaxDepth]Attrs{fs},
	}
}

func LoggerWith(l *tlog.Logger, fs Attrs) Wrapper {
	return Wrapper{
		Logger: l,
		Attrs:  [MaxDepth]Attrs{fs},
	}
}

func SpanWith(s tlog.Span, fs Attrs) SpanWrapper {
	return SpanWrapper{
		Span:  s,
		Attrs: [MaxDepth]Attrs{fs},
	}
}

func Printw(msg string, fs Attrs) {
	printw(tlog.DefaultLogger, tlog.ID{}, msg, [MaxDepth]Attrs{}, fs)
}

func (w Wrapper) With(fs Attrs) Wrapper {
	if len(fs) == 0 {
		return w
	}

	n := Wrapper{
		Logger: tlog.DefaultLogger,
	}

	par := 0
	for w.Attrs[par] != nil {
		par++
	}

	copy(n.Attrs[:], w.Attrs[:par])

	n.Attrs[par] = fs

	return n
}

func (w Wrapper) Printw(msg string, fs Attrs) {
	printw(w.Logger, tlog.ID{}, msg, w.Attrs, fs)
}

func (w SpanWrapper) With(fs Attrs) SpanWrapper {
	if len(fs) == 0 {
		return w
	}

	n := SpanWrapper{
		Span: w.Span,
	}

	par := 0
	for w.Attrs[par] != nil {
		par++
	}

	copy(n.Attrs[:], w.Attrs[:par])

	n.Attrs[par] = fs

	return n
}

func (w SpanWrapper) Printw(msg string, fs Attrs) {
	printw(w.Logger, w.ID, msg, w.Attrs, fs)
}

func printw(l *tlog.Logger, sid tlog.ID, msg string, pfs [MaxDepth]Attrs, fs Attrs) {
	if l == nil {
		return
	}

	n := 0

	par := 0
	for pfs[par] != nil {
		n += len(pfs[par])

		par++
	}

	n += len(fs)

	var kv Attrs

	if n != 0 {
		kv = make(Attrs, 0, n)

		for _, fs := range pfs[:par] {
			kv = append(kv, fs...)
		}

		kv = append(kv, fs...)
	}

	tlog.Span{Logger: l, ID: sid}.Printw(msg, kv)
}
