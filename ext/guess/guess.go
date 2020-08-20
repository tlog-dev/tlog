package guess

import (
	"sync"
	_ "unsafe"

	"github.com/nikandfor/goid"

	"github.com/nikandfor/tlog"
)

type (
	ID       = tlog.ID
	Logger   = tlog.Logger
	Span     = tlog.Span
	Location = tlog.Location

	key struct {
		goid int64
		pc   Location
	}
)

var (
	mu sync.Mutex
	c  = map[key]tlog.ID{}

	tl *Logger
)

func StartDef() Span {
	return newspan(tlog.DefaultLogger, false, ID{})
}

func SpawnDef() Span {
	return newspan(tlog.DefaultLogger, true, ID{})
}

func SpawnDefFrom(par ID) Span {
	return newspan(tlog.DefaultLogger, false, par)
}

func Start(l *Logger) Span {
	return newspan(l, false, ID{})
}

func Spawn(l *Logger) Span {
	return newspan(l, true, ID{})
}

func SpawnFrom(l *Logger, par ID) Span {
	return newspan(l, false, par)
}

func newspan(l *Logger, search bool, par ID) (s Span) {
	var loc Location
	goid := goid.ID()

	if search {
		var pc [20]Location

		st := tlog.FillStackTrace(1, pc[:])

		mu.Lock()
		for _, loc := range st {
			p, ok := c[key{goid: goid, pc: loc.Entry()}]
			if ok {
				par = p
				break
			}
		}
		mu.Unlock()

		loc = st[0].Entry()
	} else {
		loc = tlog.Funcentry(2)
	}

	s = tlog.NewSpan(l, par, 2)

	mu.Lock()
	c[key{goid: goid, pc: loc}] = s.ID
	mu.Unlock()

	return
}

func Finish(s Span) {
	s.Finish()

	goid := goid.ID()
	loc := tlog.Funcentry(1)

	mu.Lock()
	delete(c, key{goid: goid, pc: loc})
	mu.Unlock()
}
