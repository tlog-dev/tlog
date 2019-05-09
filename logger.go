package tlog

import (
	"fmt"
	"time"

	"github.com/nikandfor/json"
)

type Logger struct {
	w Writer
}

func logf(f string, args ...interface{}) Log {
	return Log{
		Start:    time.Duration(now().UnixNano()),
		Location: location(2),
		Fmt:      f,
		Args:     args,
	}
}

func NewLogger(w Writer) *Logger {
	return &Logger{
		w: w,
	}
}

func (l *Logger) Logf(f string, args ...interface{}) {
	log := logf(f, args...)
	l.w.Log(&log)
}

func (l *Logger) Start() Span {
	return newSpan(l, TraceID(rnd.Int63()), 0)
}

func (l *Logger) Spawn(id FullID) Span {
	return newSpan(l, id.TraceID, id.SpanID)
}

func (l *Logger) writeLog(log *Log) {
	l.w.Log(log)
}

func (l *Logger) writeSpan(s *Span) {
	l.w.Span(s)
}

func (l *Log) MarshalJSON(w *json.Writer) error {
	w.ObjStart()

	w.ObjKey([]byte("st"))
	fmt.Fprintf(w, "%d", int64(l.Start))

	w.ObjKey([]byte("loc"))
	fmt.Fprintf(w, "%d", l.Location)

	w.ObjKey([]byte("msg"))
	fmt.Fprintf(w, l.Fmt, l.Args...)

	w.ObjEnd()

	return w.Err()
}
