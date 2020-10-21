package tlog

import (
	"io"
	"math/rand"
	"os"
	"sync"
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

		NoTime   bool
		NoCaller bool

		NewID func() ID
	}

	Span struct {
		Logger *Logger
		ID     ID
	}

	KVs []KV

	KV struct {
		K string
		V interface{}
	}

	M map[string]interface{}

	Option func(*Logger)

	concurrentRand struct {
		mu sync.Mutex
		r  *rand.Rand
	}

	ReaderFunc func(p []byte) (int, error)
)

var ( // now, rand
	now = time.Now

	rnd = &concurrentRand{r: rand.New(rand.NewSource(time.Now().UnixNano()))} //nolint:gosec
)

// DefaultLogger is a package interface Logger object.
var DefaultLogger = func() *Logger { l := New(os.Stderr); l.NoCaller = true; return l }()

func newspan(*Logger, int, ID) Span { return Span{} }

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
	tm, pc := l.tmpc()
	ev(l, ID{}, tm, pc, 0, 0, f, args, nil)
}

func (l *Logger) Printw(f string, kvs KVs) {
	tm, pc := l.tmpc()
	ev(l, ID{}, tm, pc, 0, 0, f, nil, kvs)
}

func (s Span) Printf(f string, args ...interface{}) {
	tm, pc := s.Logger.tmpc()
	ev(s.Logger, s.ID, tm, pc, 0, 0, f, args, nil)
}

func (s Span) Printw(f string, kvs KVs) {
	tm, pc := s.Logger.tmpc()
	ev(s.Logger, s.ID, tm, pc, 0, 0, f, nil, kvs)
}

func (l *Logger) tmpc() (tm int64, pc PC) {
	if !l.NoTime {
		tm = nanotime()
	}
	if !l.NoCaller {
		pc = Caller(2)
	}
	return
}
