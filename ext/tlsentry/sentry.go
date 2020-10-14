package tlsentry

import (
	"strings"
	"time"

	"github.com/getsentry/sentry-go"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/parse"
)

type (
	ID     = tlog.ID
	PC     = tlog.PC
	Labels = tlog.Labels
	Level  = tlog.Level

	Writer struct {
		parse.DiscardWriter

		MinLevel Level

		cl *sentry.Client

		fs map[uint64]parse.Frame
		ls Labels
		ss map[ID]*span
	}

	span struct {
		par ID

		loc uint64
		st  int64
		ls  Labels
	}
)

var _ parse.Writer = &Writer{}

func New(ops sentry.ClientOptions) (*Writer, error) {
	if ops.Integrations == nil {
		ops.Integrations = func([]sentry.Integration) []sentry.Integration { return nil }
	}

	cl, err := sentry.NewClient(ops)
	if err != nil {
		return nil, err
	}

	return &Writer{
		cl: cl,
		fs: make(map[uint64]parse.Frame),
		ss: make(map[ID]*span),
	}, nil
}

func (w *Writer) Frame(f parse.Frame) error {
	w.fs[f.PC] = f

	return nil
}

func (w *Writer) SpanStarted(s parse.SpanStart) error {
	w.ss[s.ID] = &span{
		par: s.Parent,
		loc: s.PC,
		st:  s.StartedAt,
	}

	return nil
}

func (w *Writer) SpanFinished(f parse.SpanFinish) error {
	delete(w.ss, f.ID)

	return nil
}

func (w *Writer) Labels(ls parse.Labels) error {
	if ls.Span == (ID{}) {
		w.ls = ls.Labels

		return nil
	}

	sp, ok := w.ss[ls.Span]
	if !ok {
		sp = &span{}
		w.ss[ls.Span] = sp
	}

	sp.ls.Merge(ls.Labels)

	return nil
}

func (w *Writer) Message(m parse.Message) error {
	if m.Level < w.MinLevel {
		return nil
	}

	ev := sentry.Event{
		Message:   m.Text,
		Timestamp: time.Unix(0, m.Time),
		Tags:      make(map[string]string),
		Logger:    "tlog",
	}

	switch m.Level {
	case tlog.InfoLevel:
		ev.Level = sentry.LevelInfo
	case tlog.WarnLevel:
		ev.Level = sentry.LevelWarning
	case tlog.ErrorLevel:
		ev.Level = sentry.LevelError
	case tlog.FatalLevel:
		ev.Level = sentry.LevelFatal
	default:
		if m.Level > 0 {
			ev.Level = sentry.LevelFatal
		} else {
			ev.Level = sentry.LevelDebug
		}
	}

	ev.Tags = tags(w.ls, nil)

	w.addEnvironment(&ev)

	w.addTransactionInfo(&ev, m)

	return nil
}

func (w *Writer) addTransactionInfo(ev *sentry.Event, m parse.Message) {
	if m.Span == (ID{}) {
		return
	}

	s, ok := w.ss[m.Span]
	if !ok {
		return
	}

	ev.Tags = tags(s.ls, ev.Tags)

	ev.Transaction = m.Span.FullString()
	ev.StartTimestamp = time.Unix(s.st, 0)

	root := m.Span
	for root != (ID{}) {
		s, ok := w.ss[root]
		if !ok {
			break
		}

		if s.par == (ID{}) {
			break
		}

		root = s.par
	}

	id := m.Span
	for id != (ID{}) {
		s, ok := w.ss[id]
		if !ok {
			break
		}

		sp := &sentry.Span{
			TraceID:        root.FullString(),
			SpanID:         id.FullString(),
			ParentSpanID:   s.par.FullString(),
			StartTimestamp: time.Unix(s.st, 0),
			// EndTimestamp
		}

		fr := w.fs[s.loc]
		sp.Op = fr.Name
		sp.Tags = tags(s.ls, nil)

		ev.Spans = append(ev.Spans, sp)
	}
}

func (w *Writer) addEnvironment(ev *sentry.Event) {
}

func tags(ls tlog.Labels, a map[string]string) (r map[string]string) {
	if len(ls) == 0 {
		return a
	}

	r = a
	if r == nil {
		r = make(map[string]string)
	}

	for _, l := range ls {
		p := strings.Index(l, "=")

		if p == -1 {
			r[l] = ""
		} else {
			r[l[:p]] = l[p+1:]
		}
	}

	return r
}
