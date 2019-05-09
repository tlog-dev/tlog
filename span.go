package tlog

import (
	"fmt"
	"time"

	"github.com/nikandfor/json"
)

type Span struct {
	logger *Logger

	ID     FullID
	Parent SpanID

	Location Location

	Logs []Log

	Start   time.Time
	Elapsed time.Duration

	Marks int

	noCopy
}

func newSpan(l *Logger, tid TraceID, par SpanID) Span {
	return Span{
		logger: l,
		ID: FullID{
			TraceID: tid,
			SpanID:  SpanID(rnd.Int63()),
		},
		Parent:   par,
		Location: funcentry(2),
		Start:    now(),
	}
}

func (id FullID) Spawn() Span {
	return newSpan(Root, id.TraceID, id.SpanID)
}

func (s *Span) Finish() {
	if s.Elapsed != 0 {
		panic("double finish")
	}
	t := now()
	s.Elapsed = t.Sub(s.Start)
	s.logger.writeSpan(s)
}

func (s *Span) Logf(f string, args ...interface{}) {
	t := now()
	s.Logs = append(s.Logs, Log{
		Start:    t.Sub(s.Start),
		Location: location(1),
		Fmt:      f,
		Args:     args,
	})
}

func (s Span) MarshalJSON(w *json.Writer) error {
	w.ObjStart()

	w.ObjKey([]byte("tr"))
	w.StringString(s.ID.TraceID.String())

	w.ObjKey([]byte("id"))
	w.StringString(s.ID.SpanID.String())

	if s.Parent != 0 {
		w.ObjKey([]byte("par"))
		w.StringString(s.Parent.String())
	}

	w.ObjKey([]byte("loc"))
	fmt.Fprintf(w, "%d", s.Location)

	w.ObjKey([]byte("st"))
	fmt.Fprintf(w, "%d", s.Start.UnixNano())

	w.ObjKey([]byte("el"))
	fmt.Fprintf(w, "%d", s.Elapsed.Nanoseconds())

	if s.Marks != 0 {
		w.ObjKey([]byte("m"))
		fmt.Fprintf(w, "%d", s.Marks)
	}

	w.ObjKey([]byte("logs"))

	w.ArrayStart()
	for _, l := range s.Logs {
		l.MarshalJSON(w)
	}
	w.ArrayEnd()

	w.ObjEnd()

	return w.Err()
}
