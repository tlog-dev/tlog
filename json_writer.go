package tlog

import (
	"sync"

	"github.com/nikandfor/json"
)

type JSONWriter struct {
	mu   sync.Mutex
	w    *json.Writer
	locs map[Location]struct{}
}

func NewJSONWriter(w *json.Writer) *JSONWriter {
	return &JSONWriter{
		w:    w,
		locs: make(map[Location]struct{}),
	}
}

func (w *JSONWriter) Span(s *Span) {
	defer w.mu.Unlock()
	w.mu.Lock()

	w.checkLocation(s.Location)
	for _, l := range s.Logs {
		w.checkLocation(l.Location)
	}

	w.w.ObjStart()
	w.w.ObjKey([]byte("s"))
	w.w.Marshal(s)
	w.w.ObjEnd()
	w.w.NewLine()

	if err := w.w.Err(); err != nil {
		ConsoleLogger.Logf("Failed to marshal span: %v\n%v", err, s)
		return
	}
}

func (w *JSONWriter) Log(l *Log) {
	defer w.mu.Unlock()
	w.mu.Lock()

	w.checkLocation(l.Location)

	w.w.ObjStart()
	w.w.ObjKey([]byte("l"))
	w.w.Marshal(l)
	w.w.ObjEnd()
	w.w.NewLine()
	if err := w.w.Err(); err != nil {
		ConsoleLogger.Logf("Failed to marshal log: %v\n%v", err, l)
		return
	}
}

func (w *JSONWriter) Flush() error {
	return w.w.Flush()
}

func (w *JSONWriter) checkLocation(l Location) {
	if _, ok := w.locs[l]; ok {
		return
	}

	w.w.ObjStart()
	w.w.ObjKey([]byte("loc"))
	w.w.Marshal(l)
	w.w.ObjEnd()
	w.w.NewLine()

	w.locs[l] = struct{}{}
}
