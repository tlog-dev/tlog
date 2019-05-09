package tlog

import (
	"fmt"
	"math/rand"
	"os"
	"time"
)

type (
	TraceID int64
	SpanID  int64

	FullID struct {
		TraceID
		SpanID
	}

	Writer interface {
		Span(*Span)
		Log(*Log)
	}

	Log struct {
		Start    time.Duration
		Location Location
		Fmt      string
		Args     []interface{}
	}

	noCopy struct{}
)

var (
	now = func() time.Time {
		return time.Now()
	}
	rnd = rand.New(rand.NewSource(now().UnixNano()))
)

var (
	ConsoleLogger = NewLogger(NewConsoleWriter(os.Stderr))
	Root          = ConsoleLogger
)

func Logf(f string, args ...interface{}) {
	log := logf(f, args...)
	Root.writeLog(&log)
}

func Start() Span {
	return newSpan(Root, TraceID(rnd.Int63()), 0)
}

func (*noCopy) Lock() {}

func (i TraceID) String() string {
	return fmt.Sprintf("%016x", uint64(i))
}

func (i SpanID) String() string {
	return fmt.Sprintf("%016x", uint64(i))
}
