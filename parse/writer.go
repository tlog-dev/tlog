package parse

import (
	"io"

	"github.com/nikandfor/tlog"
)

type (
	Writer interface {
		Labels(Labels) error
		Frame(Frame) error
		Message(Message) error
		Metric(Metric) error
		Meta(Meta) error
		SpanStart(SpanStart) error
		SpanFinish(SpanFinish) error
	}

	AnyWriter struct {
		w tlog.Writer
	}

	ConsoleWriter struct {
		w *tlog.ConsoleWriter
	}

	ConvertWriter struct {
		w  Writer
		ls map[tlog.PC]struct{}
	}
)

var _ tlog.Writer = &ConvertWriter{}

func NewAnyWiter(w tlog.Writer) AnyWriter {
	return AnyWriter{w: w}
}

func (w AnyWriter) Labels(ls Labels) error {
	return w.w.Labels(ls.Labels, ls.Span)
}

func (w AnyWriter) Frame(l Frame) error {
	tlog.PC(l.PC).SetCache(l.Name, l.File, l.Line)

	return nil
}

func (w AnyWriter) Meta(m Meta) error {
	return w.w.Meta(
		tlog.Meta{
			Type: m.Type,
			Data: m.Data,
		},
	)
}

func (w AnyWriter) Message(m Message) error {
	return w.w.Message(
		tlog.Message{
			PC:   tlog.PC(m.PC),
			Time: m.Time,
			Text: m.Text,
		},
		m.Span,
	)
}

func (w AnyWriter) Metric(m Metric) error {
	return w.w.Metric(
		tlog.Metric{
			Name:  m.Name,
			Value: m.Value,
		},
		m.Span,
	)
}

func (w AnyWriter) SpanStart(s SpanStart) error {
	return w.w.SpanStarted(tlog.SpanStart{
		ID:        s.ID,
		Parent:    s.Parent,
		StartedAt: s.StartedAt,
		PC:        tlog.PC(s.PC),
	})
}

func (w AnyWriter) SpanFinish(f SpanFinish) error {
	return w.w.SpanFinished(tlog.SpanFinish{
		ID:      f.ID,
		Elapsed: f.Elapsed,
	})
}

func NewConsoleWriter(w io.Writer, f int) *ConsoleWriter {
	return &ConsoleWriter{
		w: tlog.NewConsoleWriter(w, f),
	}
}

func (w *ConsoleWriter) Labels(ls Labels) error {
	return w.w.Labels(ls.Labels, ls.Span)
}

func (w *ConsoleWriter) Frame(l Frame) error {
	tlog.PC(l.PC).SetCache(l.Name, l.File, l.Line)

	return nil
}

func (w *ConsoleWriter) Meta(m Meta) error {
	return w.w.Meta(
		tlog.Meta{
			Type: m.Type,
			Data: m.Data,
		},
	)
}

func (w *ConsoleWriter) Message(m Message) (err error) {
	return w.w.Message(
		tlog.Message{
			PC:   tlog.PC(m.PC),
			Time: m.Time,
			Text: m.Text,
		},
		m.Span,
	)
}

func (w *ConsoleWriter) Metric(m Metric) (err error) {
	return w.w.Metric(
		tlog.Metric{
			Name:   m.Name,
			Value:  m.Value,
			Labels: m.Labels,
		},
		m.Span,
	)
}

func (w *ConsoleWriter) SpanStart(s SpanStart) (err error) {
	return w.w.SpanStarted(tlog.SpanStart{
		ID:        s.ID,
		Parent:    s.Parent,
		StartedAt: s.StartedAt,
		PC:        tlog.PC(s.PC),
	})
}

func (w *ConsoleWriter) SpanFinish(f SpanFinish) (err error) {
	return w.w.SpanFinished(tlog.SpanFinish{
		ID:      f.ID,
		Elapsed: f.Elapsed,
	})
}

func NewConvertWriter(w Writer) *ConvertWriter {
	return &ConvertWriter{
		w:  w,
		ls: make(map[tlog.PC]struct{}),
	}
}

func (w *ConvertWriter) Labels(ls tlog.Labels, sid ID) error {
	return w.w.Labels(Labels{Labels: ls, Span: sid})
}

func (w *ConvertWriter) Meta(m tlog.Meta) error {
	return w.w.Meta(
		Meta{
			Type: m.Type,
			Data: m.Data,
		},
	)
}

func (w *ConvertWriter) Message(m tlog.Message, sid tlog.ID) error {
	err := w.location(m.PC)
	if err != nil {
		return err
	}

	return w.w.Message(Message{
		Span: sid,
		PC:   uint64(m.PC),
		Time: m.Time,
		Text: m.Text,
	})
}

func (w *ConvertWriter) Metric(m tlog.Metric, sid tlog.ID) error {
	return w.w.Metric(Metric{
		Span:   sid,
		Name:   m.Name,
		Value:  m.Value,
		Labels: m.Labels,
	})
}

func (w *ConvertWriter) SpanStarted(s tlog.SpanStart) error {
	err := w.location(s.PC)
	if err != nil {
		return err
	}

	return w.w.SpanStart(SpanStart{
		ID:        s.ID,
		Parent:    s.Parent,
		PC:        uint64(s.PC),
		StartedAt: s.StartedAt,
	})
}

func (w *ConvertWriter) SpanFinished(f tlog.SpanFinish) error {
	return w.w.SpanFinish(SpanFinish{
		ID:      f.ID,
		Elapsed: f.Elapsed,
	})
}

func (w *ConvertWriter) location(l tlog.PC) error {
	if _, ok := w.ls[l]; ok {
		return nil
	}

	name, file, line := l.NameFileLine()

	err := w.w.Frame(Frame{
		PC:   uint64(l),
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
