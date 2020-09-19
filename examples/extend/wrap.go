package extend

import (
	"sort"

	"github.com/nikandfor/tlog"
)

const MaxDepth = 4

type (
	Wrapper struct {
		*tlog.Logger
		Fields [MaxDepth]Fields // avoid allocations
	}

	SpanWrapper struct {
		tlog.Span
		Fields [MaxDepth]Fields // avoid allocations
	}

	Fields map[string]interface{}

	byKey []interface{}
)

func With(fs Fields) Wrapper {
	return Wrapper{
		Logger: tlog.DefaultLogger,
		Fields: [MaxDepth]Fields{fs},
	}
}

func LoggerWith(l *tlog.Logger, fs Fields) Wrapper {
	return Wrapper{
		Logger: l,
		Fields: [MaxDepth]Fields{fs},
	}
}

func SpanWith(s tlog.Span, fs Fields) SpanWrapper {
	return SpanWrapper{
		Span:   s,
		Fields: [MaxDepth]Fields{fs},
	}
}

func Printw(msg string, fs Fields) {
	printw(tlog.DefaultLogger, tlog.ID{}, msg, [MaxDepth]Fields{}, fs)
}

func (w Wrapper) With(fs Fields) Wrapper {
	if len(fs) == 0 {
		return w
	}

	n := Wrapper{
		Logger: tlog.DefaultLogger,
	}

	par := 0
	for w.Fields[par] != nil {
		par++
	}

	copy(n.Fields[:], w.Fields[:par])

	n.Fields[par] = fs

	return n
}

func (w Wrapper) Printw(msg string, fs Fields) {
	printw(w.Logger, tlog.ID{}, msg, w.Fields, fs)
}

func (w SpanWrapper) With(fs Fields) SpanWrapper {
	if len(fs) == 0 {
		return w
	}

	n := SpanWrapper{
		Span: w.Span,
	}

	par := 0
	for w.Fields[par] != nil {
		par++
	}

	copy(n.Fields[:], w.Fields[:par])

	n.Fields[par] = fs

	return n
}

func (w SpanWrapper) Printw(msg string, fs Fields) {
	printw(w.Logger, w.ID, msg, w.Fields, fs)
}

func printw(l *tlog.Logger, sid tlog.ID, msg string, pfs [MaxDepth]Fields, fs Fields) {
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

	var kv []interface{}

	if n != 0 {
		kv = make([]interface{}, 2*n)

		i := 0
		for _, fs := range pfs[:par] {
			st := i

			for k, v := range fs {
				kv[i] = k
				kv[i+1] = v
				i += 2
			}

			sort.Sort(byKey(kv[st:i]))
		}

		st := i

		for k, v := range fs {
			kv[i] = k
			kv[i+1] = v
			i += 2
		}

		sort.Sort(byKey(kv[st:i]))
	}

	tlog.Span{Logger: l, ID: sid}.Printw(msg, kv...)
}

func (s byKey) Len() int           { return len(s) / 2 }
func (s byKey) Less(i, j int) bool { return s[2*i].(string) < s[2*j].(string) }
func (s byKey) Swap(i, j int) {
	s[2*i], s[2*j] = s[2*j], s[2*i]
	s[2*i+1], s[2*j+1] = s[2*j+1], s[2*i+1]
}
