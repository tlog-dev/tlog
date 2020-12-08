package tlog

type (
	DiscardWriter struct{}

	TeeWriter []Writer
)

var Discard DiscardWriter

func (DiscardWriter) Write(ev *Event) error { return nil }

func NewTeeWriter(ws ...Writer) TeeWriter {
	if len(ws) != 0 {
		if w, ok := ws[0].(TeeWriter); ok {
			return append(w, ws[1:]...)
		}
	}

	return ws
}

func (w TeeWriter) Write(ev *Event) (err error) {
	for _, w := range w {
		e := w.Write(ev)
		if err == nil {
			err = e
		}
	}

	return
}
