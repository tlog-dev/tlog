package tlog

import (
	"io"
	"os"
	"sync"
	"time"
)

type (
	ID [16]byte

	Type byte

	Level int8

	Logger struct {
		mu sync.Mutex
		encoder

		io.Writer

		NoTime   bool
		NoCaller bool

		NewID func() ID
	}

	Span struct {
		Logger *Logger
		ID     ID
	}

	Event struct {
		Logger *Logger
		ID     ID
	}

	Option func(l *Logger)
)

const (
	Info = iota
	Warn
	Error
	Fatal

	Debug = -1
)

var unixnow = fastnow

var DefaultLogger = New(os.Stderr, WithNoCaller)

func newspan(l *Logger, d int, par ID) Span { return Span{} }

func New(w io.Writer, ops ...Option) *Logger {
	l := &Logger{
		Writer: w,
		NewID:  MathRandID,
	}

	for _, o := range ops {
		o(l)
	}

	return l
}

func (l *Logger) Printf(f string, args ...interface{}) {
	if l == nil {
		return
	}

	l.mu.Lock()

	if !l.NoTime {
		l.timestamp(unixnow())
	}
	if !l.NoCaller {
		//	var pc PC
		//	caller1(1, &pc, 1, 1)
		//	l.caller(pc)

		//	l.caller(Caller(1))
	}

	l.message(f, args)

	l.write()

	l.mu.Unlock()
}

func (l *Logger) Printw(msg string, kvs ...interface{}) {
	if l == nil {
		return
	}

	l.mu.Lock()

	if !l.NoTime {
		l.timestamp(unixnow())
	}
	if !l.NoCaller {
		//	l.caller(Caller(1))
	}

	l.message(msg, nil)

	l.kvs(kvs...)

	l.write()

	l.mu.Unlock()
}

func (l *Logger) Start(name ...string) (s Span) {
	if l == nil {
		return
	}

	s.Logger = l
	s.ID = l.NewID()

	l.mu.Lock()

	l.id(s.ID)

	if !l.NoTime {
		l.timestamp(unixnow())
	}
	if !l.NoCaller {
		//	l.caller(Caller(1))
	}

	l.rectype('s')

	if len(name) != 0 {
		l.message(name[0], nil)
	}

	l.write()

	l.mu.Unlock()

	return s
}

func (l *Logger) Spawn(par ID, name ...string) (s Span) {
	if l == nil {
		return
	}

	s.Logger = l
	s.ID = l.NewID()

	l.mu.Lock()

	l.id(s.ID)

	if !l.NoTime {
		l.timestamp(unixnow())
	}
	if !l.NoCaller {
		//	l.caller(Caller(1))
	}

	l.rectype('s')

	if len(name) != 0 {
		l.message(name[0], nil)
	}

	l.parent(par)

	l.write()

	l.mu.Unlock()

	return s
}

func (l *Logger) write() (err error) {
	l.eor()

	_, err = l.Writer.Write(l.encoder.b)

	l.encoder.reset()

	return
}

func (l *Logger) Event() Event {
	if l == nil {
		return Event{}
	}

	l.mu.Lock()

	return Event{Logger: l}
}

func (ev Event) Write() (err error) {
	if ev.Logger != nil {
		err = ev.Logger.write()

		ev.Logger.mu.Unlock()
	}

	return
}

func (l *Logger) reset() {
	l.encoder.reset()

	l.mu.Unlock()
}

func (ev Event) TimeNow() Event {
	if ev.Logger != nil {
		ev.Logger.timestamp(unixnow())
	}

	return ev
}

func (ev Event) Timestamp(ts int64) Event {
	if ev.Logger != nil {
		ev.Logger.timestamp(ts)
	}

	return ev
}

func (ev Event) Time(t time.Time) Event {
	if ev.Logger != nil {
		ev.Logger.timestamp(t.UnixNano())
	}

	return ev
}
