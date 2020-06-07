package parse

import (
	"io"
	"time"

	"github.com/nikandfor/tlog"
)

type (
	Writer interface {
		Labels(Labels) error
		Location(Location) error
		Message(Message) error
		SpanStart(SpanStart) error
		SpanFinish(SpanFinish) error
	}

	AnyWriter struct {
		w tlog.Writer
	}

	ConsoleWriter struct {
		w       *tlog.ConsoleWriter
		started map[ID]time.Time
	}
)

func NewAnyWiter(w tlog.Writer) AnyWriter {
	return AnyWriter{w: w}
}

func (w AnyWriter) Labels(ls Labels) error {
	return w.w.Labels(ls)
}

func (w AnyWriter) Location(l Location) error {
	tlog.Location(l.PC).SetCache(l.Name, l.File, l.Line)

	return nil
}

func (w AnyWriter) Message(m Message) error {
	w.w.Message(
		tlog.Message{
			Location: tlog.Location(m.Location),
			Time:     m.Time,
			Format:   m.Text,
		},
		tlog.Span{
			ID: m.Span,
		},
	)

	return nil
}

func (w AnyWriter) SpanStart(s SpanStart) error {
	w.w.SpanStarted(
		tlog.Span{
			ID:      s.ID,
			Started: s.Started,
		},
		s.Parent,
		tlog.Location(s.Location),
	)

	return nil
}

func (w AnyWriter) SpanFinish(f SpanFinish) error {
	w.w.SpanFinished(
		tlog.Span{
			ID: f.ID,
		},
		f.Elapsed,
	)

	return nil
}

func NewConsoleWriter(w io.Writer, f int) *ConsoleWriter {
	return &ConsoleWriter{
		w:       tlog.NewConsoleWriter(w, f),
		started: make(map[ID]time.Time),
	}
}

func (w *ConsoleWriter) Labels(ls Labels) error {
	w.w.Message(
		tlog.Message{
			Format: "Labels: %q",
			Args:   []interface{}{ls},
		},
		tlog.Span{},
	)

	return nil
}

func (w *ConsoleWriter) Location(l Location) error {
	tlog.Location(l.PC).SetCache(l.Name, l.File, l.Line)

	return nil
}

func (w *ConsoleWriter) Message(m Message) error {
	w.w.Message(
		tlog.Message{
			Location: tlog.Location(m.Location),
			Time:     m.Time,
			Format:   m.Text,
		},
		tlog.Span{
			ID:      m.Span,
			Started: w.started[m.Span],
		},
	)

	return nil
}

func (w *ConsoleWriter) SpanStart(s SpanStart) error {
	w.w.SpanStarted(
		tlog.Span{
			ID:      s.ID,
			Started: s.Started,
		},
		s.Parent,
		tlog.Location(s.Location),
	)

	w.started[s.ID] = s.Started

	return nil
}

func (w *ConsoleWriter) SpanFinish(f SpanFinish) error {
	w.w.SpanFinished(
		tlog.Span{
			ID:      f.ID,
			Started: w.started[f.ID],
		},
		f.Elapsed,
	)

	delete(w.started, f.ID)

	return nil
}
