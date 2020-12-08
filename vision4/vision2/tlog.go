package tlog

import (
	"io"
	"math/rand"
	"time"
)

type (
	ID [16]byte

	Type byte

	// Level is log level.
	// Default Level is 0. The higher level the more important the message.
	// The lower level the less important the message.
	Level int8

	Logger struct {
		io.Writer
		enc encoder

		AddTime   bool
		AddCaller bool
	}

	Span struct {
		Logger *Logger
		ID     ID
	}

	KV map[string]interface{}

	Option func(*Logger)
)

var ( // now, rand
	now = time.Now

	rnd = &concurrentRand{r: rand.New(rand.NewSource(time.Now().UnixNano()))} //nolint:gosec
)

func New(w io.Writer, ops ...Option) *Logger {
	l := &Logger{
		Writer: w,
	}

	for _, o := range ops {
		o(l)
	}

	return l
}

func (l *Logger) Printf(f string, args ...interface{}) {
	ev(l, ID{}, 0, 0, 0, f, args, nil)
}

func (s Span) Printf(f string, args ...interface{}) {
	ev(s.Logger, s.ID, 0, 0, 0, f, args, nil)
}

func (s Span) Printw(f string, kv KV) {
	ev(s.Logger, s.ID, 0, 0, 0, f, nil, kv)
}
