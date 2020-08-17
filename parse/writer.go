package parse

import (
	"fmt"
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

	ConvertWriter struct {
		w  Writer
		ls map[tlog.Location]struct{}
	}
)

func NewAnyWiter(w tlog.Writer) AnyWriter {
	return AnyWriter{w: w}
}

func (w AnyWriter) Labels(ls Labels) error {
	return w.w.Labels(ls.Labels, ls.Span)
}

func (w AnyWriter) Location(l Location) error {
	tlog.Location(l.PC).SetCache(l.Name, l.File, l.Line)

	return nil
}

func (w AnyWriter) Message(m Message) error {
	return w.w.Message(
		tlog.Message{
			Location: tlog.Location(m.Location),
			Time:     m.Time,
			Format:   m.Text,
		},
		tlog.Span{
			ID: m.Span,
		},
	)
}

func (w AnyWriter) SpanStart(s SpanStart) error {
	return w.w.SpanStarted(
		tlog.Span{
			ID:      s.ID,
			Started: s.Started,
		},
		s.Parent,
		tlog.Location(s.Location),
	)
}

func (w AnyWriter) SpanFinish(f SpanFinish) error {
	return w.w.SpanFinished(
		tlog.Span{
			ID: f.ID,
		},
		f.Elapsed,
	)
}

func NewConsoleWriter(w io.Writer, f int) *ConsoleWriter {
	return &ConsoleWriter{
		w:       tlog.NewConsoleWriter(w, f),
		started: make(map[ID]time.Time),
	}
}

func (w *ConsoleWriter) Labels(ls Labels) error {
	return w.w.Message(
		tlog.Message{
			Format: "Labels: %q",
			Args:   []interface{}{ls},
		},
		tlog.Span{},
	)
}

func (w *ConsoleWriter) Location(l Location) error {
	tlog.Location(l.PC).SetCache(l.Name, l.File, l.Line)

	return nil
}

func (w *ConsoleWriter) Message(m Message) (err error) {
	return w.w.Message(
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
}

func (w *ConsoleWriter) SpanStart(s SpanStart) (err error) {
	err = w.w.SpanStarted(
		tlog.Span{
			ID:      s.ID,
			Started: s.Started,
		},
		s.Parent,
		tlog.Location(s.Location),
	)

	w.started[s.ID] = s.Started

	return
}

func (w *ConsoleWriter) SpanFinish(f SpanFinish) (err error) {
	err = w.w.SpanFinished(
		tlog.Span{
			ID:      f.ID,
			Started: w.started[f.ID],
		},
		f.Elapsed,
	)

	delete(w.started, f.ID)

	return
}

func NewConvertWriter(w Writer) *ConvertWriter {
	return &ConvertWriter{
		w:  w,
		ls: make(map[tlog.Location]struct{}),
	}
}

func (w *ConvertWriter) Labels(ls tlog.Labels, sid ID) error {
	return w.w.Labels(Labels{Labels: ls, Span: sid})
}

func (w *ConvertWriter) Message(m tlog.Message, s tlog.Span) error {
	err := w.location(m.Location)
	if err != nil {
		return err
	}

	return w.w.Message(Message{
		Span:     s.ID,
		Location: uintptr(m.Location),
		Time:     m.Time,
		Text:     fmt.Sprintf(m.Format, m.Args...),
	})
}

func (w *ConvertWriter) SpanStarted(s tlog.Span, p ID, l tlog.Location) error {
	err := w.location(l)
	if err != nil {
		return err
	}

	return w.w.SpanStart(SpanStart{
		ID:       s.ID,
		Parent:   p,
		Location: uintptr(l),
		Started:  s.Started,
	})
}

func (w *ConvertWriter) SpanFinished(s tlog.Span, el time.Duration) error {
	return w.w.SpanFinish(SpanFinish{
		ID:      s.ID,
		Elapsed: el,
	})
}

func (w *ConvertWriter) location(l tlog.Location) error {
	if _, ok := w.ls[l]; ok {
		return nil
	}

	name, file, line := l.NameFileLine()

	err := w.w.Location(Location{
		PC:   uintptr(l),
		Name: name,
		File: file,
		Line: line,
	})
	if err != nil {
		return err
	}

	w.ls[l] = struct{}{}

	return nil
}
