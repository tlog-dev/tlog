package convert

import (
	"bytes"
	"errors"
	"io"
	"sort"

	"tlog.app/go/tlog"
	"tlog.app/go/tlog/tlwire"
)

type (
	Rewriter struct {
		io.Writer

		tlwire.Decoder
		tlwire.Encoder

		Rule RewriterRule

		b []byte
	}

	RewriterRule interface {
		Rewrite(b, p []byte, path []tlog.RawMessage, kst, st int) ([]byte, int, error)
	}

	RewriterFunc func(b, p []byte, path []tlog.RawMessage, kst, st int) ([]byte, int, error)

	KeyRenamer struct {
		rules []RenameRule

		d tlwire.Decoder
		e tlwire.Encoder

		Rewriter RewriterRule
		Fallback RewriterRule
	}

	RenameRule struct {
		Path []tlog.RawMessage

		Rename []byte
		Prefix []byte
		Remove bool
	}
)

var ErrFallback = errors.New("fallback")

func NewRewriter(w io.Writer) *Rewriter {
	return &Rewriter{Writer: w}
}

func (w *Rewriter) Write(p []byte) (n int, err error) {
	w.b, n, err = w.Rewrite(w.b[:0], p, nil, -1, 0)
	if err != nil {
		return 0, err
	}

	if w.Writer != nil {
		_, err = w.Writer.Write(w.b)
	}

	return
}

func (w *Rewriter) Rewrite(b, p []byte, path []tlog.RawMessage, kst, st int) (r []byte, i int, err error) {
	if w.Rule != nil {
		r, i, err = w.Rule.Rewrite(b, p, path, kst, st)
		if !errors.Is(err, ErrFallback) {
			return
		}
	}

	if st == len(p) {
		return b, st, nil
	}

	tag, sub, i := w.Tag(p, st)

	if kst != -1 && tag != tlwire.Semantic {
		b = append(b, p[kst:st]...)
	}

	switch tag {
	case tlwire.Int, tlwire.Neg:
	case tlwire.String, tlwire.Bytes:
		i = w.Skip(p, st)
	case tlwire.Array, tlwire.Map:
		b = append(b, p[st:i]...)
		kst := -1
		subp := path

		if tag == tlwire.Array {
			subp = append(subp, []byte{tlwire.Array})
		}

		for el := 0; sub == -1 || el < int(sub); el++ {
			if sub == -1 && w.Break(p, &i) {
				break
			}

			if tag == tlwire.Map {
				kst = i
				_, i = w.Bytes(p, i)

				subp = append(subp[:len(path)], p[kst:i])
			}

			b, i, err = w.Rewrite(b, p, subp, kst, i)
			if err != nil {
				return
			}
		}

		if sub == -1 {
			b = w.AppendBreak(b)
		}

		return b, i, nil
	case tlwire.Semantic:
		path = append(path, p[st:i])

		return w.Rewrite(b, p, path, kst, i)
	case tlwire.Special:
		switch sub {
		case tlwire.False,
			tlwire.True,
			tlwire.Nil,
			tlwire.Undefined,
			tlwire.None,
			tlwire.Hidden,
			tlwire.SelfRef,
			tlwire.Break:
		case tlwire.Float8:
			i += 1
		case tlwire.Float16:
			i += 2
		case tlwire.Float32:
			i += 4
		case tlwire.Float64:
			i += 8
		default:
			panic("unsupported special")
		}
	}

	b = append(b, p[st:i]...)

	return b, i, nil
}

func (f RewriterFunc) Rewrite(b, p []byte, path []tlog.RawMessage, kst, st int) ([]byte, int, error) {
	return f(b, p, path, kst, st)
}

func NewKeyRenamer(rew RewriterRule, rules ...RenameRule) *KeyRenamer {
	w := &KeyRenamer{
		Rewriter: rew,
	}

	w.Append(rules...)

	return w
}

func (w *KeyRenamer) Append(rules ...RenameRule) {
	w.rules = append(w.rules, rules...)

	sort.Slice(w.rules, func(i, j int) bool {
		return w.cmp(w.rules[i].Path, w.rules[j].Path) < 0
	})
}

func (w *KeyRenamer) Rewrite(b, p []byte, path []tlog.RawMessage, kst, st int) ([]byte, int, error) {
	pos := sort.Search(len(w.rules), func(i int) bool {
		rule := w.rules[i]

		return w.cmp(path, rule.Path) <= 0
	})

	//	fmt.Printf("rewrite  %q -> %v %v\n", path, pos, pos < len(w.rules) && w.cmp(path, w.rules[pos].Path) == 0)

	if pos == len(w.rules) || w.cmp(path, w.rules[pos].Path) != 0 {
		return w.fallback(b, p, path, kst, st)
	}

	rule := w.rules[pos]

	if rule.Remove {
		end := w.d.Skip(p, st)
		return b, end, nil
	}

	key, kend := w.d.Bytes(p, kst)

	l := len(rule.Prefix)

	if rule.Rename != nil {
		l += len(rule.Rename)
	} else {
		l += len(key)
	}

	b = w.e.AppendTag(b, tlwire.String, l)
	b = append(b, rule.Prefix...)

	if rule.Rename != nil {
		b = append(b, rule.Rename...)
	} else {
		b = append(b, key...)
	}

	b = append(b, p[kend:st]...)

	if w.Rewriter != nil {
		return w.Rewriter.Rewrite(b, p, path, -1, st)
	}

	end := w.d.Skip(p, st)
	b = append(b, p[st:end]...)

	return b, end, nil
}

func (w *KeyRenamer) fallback(b, p []byte, path []tlog.RawMessage, kst, st int) ([]byte, int, error) {
	if w.Fallback == nil {
		return b, st, ErrFallback
	}

	return w.Fallback.Rewrite(b, p, path, kst, st)
}

func (w *KeyRenamer) cmp(x, y []tlog.RawMessage) (r int) {
	//	defer func() {
	//		fmt.Printf("cmp %q %q -> %d  from %v\n", x, y, r, loc.Caller(1))
	//	}()
	for i := 0; i < min(len(x), len(y)); i++ {
		r = bytes.Compare(x[i], y[i])
		if r != 0 {
			return r
		}
	}

	if len(x) != len(y) {
		if len(x) < len(y) {
			return -1
		}

		return 1
	}

	return 0
}

func min(a, b int) int {
	if a < b {
		return a
	}

	return b
}
