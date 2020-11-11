package tlog

import (
	"io"
	"os"
	"sync"
	"time"

	"github.com/nikandfor/tlog/low"
	"github.com/nikandfor/tlog/tlt"
	"github.com/nikandfor/tlog/tlwriter"
	"github.com/nikandfor/tlog/wire"
)

type (
	ID     = tlt.ID
	Type   = tlt.Type
	Level  = tlt.Level
	Labels = tlt.Labels

	Logger struct {
		wire.Encoder

		mu sync.Mutex

		tags []wire.Tag
		b    []byte

		NoTime   bool
		NoCaller bool

		NewID func() ID
	}

	Span struct {
		Logger    *Logger
		ID        ID
		StartedAt time.Time
	}

	Option func(l *Logger)
)

// Log Levels
const (
	Info = iota
	Warn
	Error
	Fatal

	Debug = -1
)

// Metric types
const (
	Counter   = "counter"
	Gauge     = "gauge"
	Summary   = "summary"
	Histogram = "histogram"
)

var ( // now
	unixnow = low.UnixNano
	now     = time.Now
)

var DefaultLogger = New(tlwriter.NewConsole(os.Stderr, tlwriter.LstdFlags), WithNoCaller)

func newspan(l *Logger, par ID, d int, args []interface{}) (s Span) {
	if l == nil {
		return
	}

	if !l.NoTime {
		s.StartedAt = now()
	}

	s.Logger = l
	s.ID = l.NewID()

	defer l.mu.Unlock()
	l.mu.Lock()

	l.tags = l.tags[:0]

	l.tags = wire.AppendTagVal(l.tags, wire.Span, s.ID)

	if !l.NoTime {
		l.tags = wire.AppendTagVal(l.tags, wire.Time, s.StartedAt.UnixNano())
	}

	l.tags = wire.AppendTagVal(l.tags, wire.Type, wire.Start)

	if par != (ID{}) {
		l.tags = wire.AppendTagVal(l.tags, wire.Parent, par)
	}

	if len(args) != 0 {
		if name, ok := args[0].(string); ok {
			l.tags = wire.AppendTagVal(l.tags, wire.Name, name)
			args = args[1:]
		}
	}

	wire.Event(&l.Encoder, l.tags, args)

	return
}

func newprint(l *Logger, id ID, d int16, lv Level, msg string, args []interface{}, kvs []interface{}) {
	if l == nil {
		return
	}

	defer l.mu.Unlock()
	l.mu.Lock()

	l.tags = l.tags[:0]

	if id != (ID{}) {
		l.tags = wire.AppendTagVal(l.tags, wire.Span, id)
	}

	if !l.NoTime {
		l.tags = wire.AppendTagVal(l.tags, wire.Time, unixnow())
	}
	if !l.NoCaller {
		//	var pc PC
		//	caller1(1, &pc, 1, 1)
		//	tags = append(tags, Tag{R: rLocation, V: pc})
	}

	if len(args) != 0 {
		l.tags = wire.AppendTagVal(l.tags, wire.Message, wire.Format{Fmt: msg, Args: args})
	} else {
		l.tags = wire.AppendTagVal(l.tags, wire.Message, msg)
	}

	wire.Event(&l.Encoder, l.tags, kvs)
}

func observe(l *Logger, id ID, name string, v interface{}, kvs []interface{}) {
	if l == nil {
		return
	}
	if v == nil {
		panic("nil value")
	}

	defer l.mu.Unlock()
	l.mu.Lock()

	l.tags = l.tags[:0]

	if id != (ID{}) {
		l.tags = wire.AppendTagVal(l.tags, wire.Span, id)
	}

	l.tags = wire.AppendTagVal(l.tags, wire.Name, name)
	l.tags = wire.AppendTagVal(l.tags, wire.Value, v)

	wire.Event(&l.Encoder, l.tags, kvs)
}

func New(w io.Writer, ops ...Option) *Logger {
	l := &Logger{
		NewID: tlt.MathRandID,
	}

	l.Writer = w

	for _, o := range ops {
		o(l)
	}

	return l
}

func (l *Logger) Event(tags []wire.Tag, kvs []interface{}) {
	if l == nil {
		return
	}

	defer l.mu.Unlock()
	l.mu.Lock()

	wire.Event(&l.Encoder, tags, kvs)
}

func (s Span) Event(tags []wire.Tag, kvs []interface{}) {
	l := s.Logger
	if l == nil {
		return
	}

	defer l.mu.Unlock()
	l.mu.Lock()

	l.tags = wire.AppendTagVal(l.tags[:0], wire.Span, s.ID)
	l.tags = append(l.tags, tags...)

	wire.Event(&l.Encoder, l.tags, kvs)
}

func SetLabels(ls Labels) {
	DefaultLogger.SetLabels(ls)
}

func (l *Logger) SetLabels(ls Labels) {
	defer l.mu.Unlock()
	l.mu.Lock()

	wire.Event(&l.Encoder, []wire.Tag{{T: wire.Labels, V: ls}}, nil)
}

func Start(args ...interface{}) Span {
	return newspan(DefaultLogger, ID{}, 0, args)
}

func Spawn(par ID, args ...interface{}) Span {
	return newspan(DefaultLogger, par, 0, args)
}

func (l *Logger) Start(args ...interface{}) Span {
	return newspan(l, ID{}, 0, args)
}

func (l *Logger) Spawn(par ID, args ...interface{}) Span {
	return newspan(l, par, 0, args)
}

func (s Span) Finish() {
	l := s.Logger
	if l == nil {
		return
	}

	var d time.Duration
	if s.StartedAt != (time.Time{}) {
		d = now().Sub(s.StartedAt)
	}

	defer l.mu.Unlock()
	l.mu.Lock()

	l.tags = l.tags[:0]

	l.tags = wire.AppendTagVal(l.tags, wire.Span, s.ID)

	l.tags = wire.AppendTagVal(l.tags, wire.Type, wire.Finish)

	if d != 0 {
		l.tags = wire.AppendTagVal(l.tags, wire.Duration, d)
	}

	wire.Event(&l.Encoder, l.tags, nil)
}

func Printf(f string, args ...interface{}) {
	newprint(DefaultLogger, ID{}, 0, 0, f, args, nil)
}

func Printw(msg string, kvs ...interface{}) {
	newprint(DefaultLogger, ID{}, 0, 0, msg, nil, kvs)
}

func (l *Logger) Printf(f string, args ...interface{}) {
	newprint(l, ID{}, 0, 0, f, args, nil)
}

func (l *Logger) Printw(msg string, kvs ...interface{}) {
	newprint(l, ID{}, 0, 0, msg, nil, kvs)
}

func (s Span) Printf(f string, args ...interface{}) {
	newprint(s.Logger, s.ID, 0, 0, f, args, nil)
}

func (s Span) Printw(msg string, kvs ...interface{}) {
	newprint(s.Logger, s.ID, 0, 0, msg, nil, kvs)
}

func Observe(name string, v float64, args ...interface{}) {
	observe(DefaultLogger, ID{}, name, v, args)
}

func (l *Logger) Observe(name string, v float64, args ...interface{}) {
	observe(l, ID{}, name, v, args)
}

func (s Span) Observe(name string, v float64, args ...interface{}) {
	observe(s.Logger, s.ID, name, v, args)
}

func RegisterMetric(name, typ, help string, kvs ...interface{}) {
	DefaultLogger.RegisterMetric(name, typ, help, kvs...)
}

func (l *Logger) RegisterMetric(name, typ, help string, kvs ...interface{}) {
	q := []interface{}{"type", typ, "help", help}
	q = append(q, kvs...)

	l.Event([]wire.Tag{{T: wire.Type, V: 'm'}, {T: wire.Name, V: name}}, q)
}
