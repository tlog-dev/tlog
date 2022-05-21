//go:build ignore

package tq

import (
	"context"
	"io"
	"strconv"
	"text/scanner"

	"github.com/PaesslerAG/gval"

	"github.com/nikandfor/errors"
	"github.com/nikandfor/tlog"
)

type (
	TQ struct {
		io.Writer

		ev gval.Evaluable
	}
)

var DefaultLang gval.Language = gval.NewLanguage(
	str,
	numInt,
	numFlt,
	ident,
	pipe,

	gval.Parentheses(),
)

var (
	str = gval.PrefixExtension(scanner.String, parseString)

	numInt = gval.PrefixExtension(scanner.Int, parseInt)
	numFlt = gval.PrefixExtension(scanner.Float, parseFlt)

	ident = gval.PrefixExtension(scanner.Ident, func(ctx context.Context, p *gval.Parser) (gval.Evaluable, error) {
		tk := p.TokenText()

		tlog.Printw("ident", "tk", tk)

		f, ok := filters[tk]
		if !ok {
			return nil, errors.New("unexpected token: %v", tk)
		}

		return wrap(f()), nil
	})

	pipe = gval.PostfixOperator("|", func(ctx context.Context, p *gval.Parser, pre gval.Evaluable) (gval.Evaluable, error) {
		post, err := p.ParseExpression(ctx)
		if err != nil {
			return nil, err
		}

		return func(ctx context.Context, v interface{}) (_ interface{}, err error) {
			v, err = pre(ctx, v)
			if err != nil {
				return nil, errors.Wrap(err, "%v", pre)
			}

			v, err = post(ctx, v)
			if err != nil {
				return nil, errors.Wrap(err, "%v", post)
			}

			return v, nil
		}, nil
	})

	order = []gval.Language{}
)

var (
	filters = map[string]func() Filter{
		"keys":   func() Filter { return &Keys{} },
		"cat":    func() Filter { return &Cat{} },
		"length": func() Filter { return &Length{} },
	}
)

func parseString(ctx context.Context, p *gval.Parser) (gval.Evaluable, error) {
	tk := p.TokenText()

	tlog.Printw("string", "tk", tk)

	s, err := strconv.Unquote(tk)
	if err != nil {
		return nil, errors.Wrap(err, "unquote: %s", tk)
	}

	f := &Literal{}
	f.B = f.AppendString(nil, s)

	return wrap(f), nil
}

func parseInt(ctx context.Context, p *gval.Parser) (gval.Evaluable, error) {
	tk := p.TokenText()

	tlog.Printw("integer", "tk", tk)

	x, err := strconv.ParseInt(tk, 10, 64)
	if err != nil {
		return nil, errors.Wrap(err, "parse int")
	}

	f := &Literal{}
	f.B = f.AppendInt64(nil, x)

	return wrap(f), nil
}

func parseFlt(ctx context.Context, p *gval.Parser) (gval.Evaluable, error) {
	tk := p.TokenText()

	tlog.Printw("float", "tk", tk)

	x, err := strconv.ParseFloat(tk, 64)
	if err != nil {
		return nil, errors.Wrap(err, "parse int")
	}

	f := &Literal{}
	f.B = f.AppendFloat(nil, x)

	return wrap(f), nil
}

func wrap(w Filter) gval.Evaluable {
	return func(ctx context.Context, arg interface{}) (_ interface{}, err error) {
		tlog.Printw("filter", "filter", w, "arg", arg)

		_, err = w.Write(arg.([]byte))
		tlog.Printw("filter", "filter", w, "arg", arg, "err", err, "res", w.Result())
		if err != nil {
			return nil, errors.Wrap(err, "%v", w)
		}

		return w.Result(), nil
	}
}

func op(r rune) gval.Language {
	return gval.InfixEvalOperator(string(r), func(a, b gval.Evaluable) (_ gval.Evaluable, err error) {
		x := &Op{		s.ch = s.next()
		if s.ch == '\uFEFF' {
			s.ch = s.next() // ignore BOM
		}
			Op: r,
		}
	})
}

func New(w io.Writer, q string, ls ...gval.Language) (*TQ, error) {
	l := gval.NewLanguage(append([]gval.Language{DefaultLang}, ls...)...)

	ev, err := l.NewEvaluable(q)
	if err != nil {
		return nil, err
	}

	f := &TQ{
		Writer: w,
		ev:     ev,
	}

	return f, nil
}

func (f *TQ) Write(p []byte) (int, error) {
	ctx := context.Background()

	r, err := f.ev(ctx, p)
	if err != nil {
		return 0, err
	}

	tlog.Printw("res", "r", r)

	return f.Writer.Write(r.([]byte))
}
