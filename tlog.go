package tlog

import (
	"io"
	"sync"
	"time"

	"github.com/nikandfor/loc"
	"github.com/nikandfor/tlog/low"
)

type (
	Logger struct {
		sync.Mutex

		Encoder

		NewID func() ID

		NoTime   bool
		NoCaller bool

		buf []interface{}

		//	filter *filter // accessed by atomic operations
	}

	Span struct {
		Logger    *Logger
		ID        ID
		StartedAt time.Time
	}
)

var now = time.Now

func New(w io.Writer) *Logger {
	l := &Logger{
		Encoder: Encoder{
			Writer: w,
		},
		NewID: MathRandID,
	}

	return l
}

func newmessage(l *Logger, id ID, msg interface{}, kvs []interface{}) {
	if l == nil {
		return
	}

	var t Timestamp
	if !l.NoTime {
		t = Timestamp(low.UnixNano())
	}

	var lc loc.PC
	if !l.NoCaller {
		caller1(2, &lc, 1, 1)
	}

	defer l.Unlock()
	l.Lock()

	defer l.clearBuf()

	if id != (ID{}) {
		l.appendBuf("s", id)
	}

	if !l.NoTime {
		l.appendBuf("t", t)
	}
	if !l.NoCaller {
		l.appendBuf("l", lc)
	}

	if msg != nil {
		l.appendBuf("m", msg)
	}

	_ = l.Encoder.Encode(l.buf, kvs)
}

func newspan(l *Logger, par ID, n string, kvs []interface{}) (s Span) {
	if l == nil {
		return
	}

	s.Logger = l
	s.ID = l.NewID()
	s.StartedAt = now()

	var lc loc.PC
	if !l.NoCaller {
		caller1(2, &lc, 1, 1)
	}

	defer l.Unlock()
	l.Lock()

	defer l.clearBuf()

	l.appendBuf("s", s.ID)
	if par != (ID{}) {
		l.appendBuf("p", par)
	}

	if !l.NoTime {
		l.appendBuf("t", Timestamp(s.StartedAt.UnixNano()))
	}
	if !l.NoCaller {
		l.appendBuf("l", lc)
	}

	l.appendBuf("t", "s")

	if n != "" {
		l.appendBuf("m", n)
	}

	_ = l.Encoder.Encode(l.buf, kvs)

	return
}

func (s Span) Finish(kvs ...interface{}) {
	if s.Logger == nil {
		return
	}

	var el time.Duration
	if !s.Logger.NoTime {
		el = now().Sub(s.StartedAt)
	}

	defer s.Logger.Unlock()
	s.Logger.Lock()

	defer s.Logger.clearBuf()

	s.Logger.appendBuf("s", s)
	s.Logger.appendBuf("t", "f")

	if el != 0 {
		s.Logger.appendBuf("e", el)
	}

	_ = s.Logger.Encoder.Encode(s.Logger.buf, kvs)
}

func (l *Logger) Event(kvs ...[]interface{}) error {
	if l == nil {
		return nil
	}

	defer l.Unlock()
	l.Lock()

	return l.Encoder.Encode(nil, kvs...)
}

func (s Span) Event(kvs ...[]interface{}) error {
	if s.Logger == nil {
		return nil
	}

	defer s.Logger.Unlock()
	s.Logger.Lock()

	defer s.Logger.clearBuf()

	if s.ID != (ID{}) {
		s.Logger.appendBuf("s", s.ID)
	}

	return s.Logger.Encoder.Encode(s.Logger.buf, kvs...)
}

//go:noinline
func (l *Logger) Printf(f string, args ...interface{}) {
	newmessage(l, ID{}, Format{Fmt: f, Args: args}, nil)
}

//go:noinline
func (l *Logger) Printw(msg string, kvs ...interface{}) {
	newmessage(l, ID{}, msg, kvs)
}

//go:noinline
func (s Span) Printf(f string, args ...interface{}) {
	newmessage(s.Logger, s.ID, Format{Fmt: f, Args: args}, nil)
}

//go:noinline
func (s Span) Printw(msg string, kvs ...interface{}) {
	newmessage(s.Logger, s.ID, msg, kvs)
}

func (l *Logger) Start(n string, kvs ...interface{}) Span {
	return newspan(l, ID{}, n, kvs)
}

func (l *Logger) Spawn(par ID, n string, kvs ...interface{}) Span {
	return newspan(l, par, n, kvs)
}

func (s Span) Spawn(n string, kvs ...interface{}) Span {
	return newspan(s.Logger, s.ID, n, kvs)
}

func (l *Logger) appendBuf(vals ...interface{}) {
	l.buf = append0(l.buf, vals...)
}

func append1(b []interface{}, v ...interface{}) []interface{} {
	return append(b, v...)
}

func (l *Logger) clearBuf() {
	for i := range l.buf {
		l.buf[i] = nil
	}

	l.buf = l.buf[:0]
}
