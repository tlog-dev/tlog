package tlog

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

type (
	Event1 struct {
		Context context.Context
		Logger  *Logger
		Span    ID
		Type    Type
		Level   Level
		Attrs   []A
	}

	slice struct {
		p    unsafe.Pointer
		l, c int
	}

	sorter []A
)

var (
	pool = sync.Pool{New: func() interface{} { return &Event{} }}

	pool16    = sync.Pool{New: func() interface{} { return new(int16) }}
	pool32    = sync.Pool{New: func() interface{} { return new(int32) }}
	pool64    = sync.Pool{New: func() interface{} { return new(int64) }}
	poolword  = sync.Pool{New: func() interface{} { return new(int) }}
	pooltime  = sync.Pool{New: func() interface{} { return new(time.Time) }}
	poolid    = sync.Pool{New: func() interface{} { return new(ID) }}
	poolslice = sync.Pool{New: func() interface{} { return new(slice) }}
)

func Ev(ctx context.Context, l *Logger, id ID, lv Level) (ev *Event) {
	if l == nil {
		return nil
	}

	ev = pool.Get().(*Event)

	*ev = Event{
		Context: ctx,
		Logger:  l,
		Span:    id,
		Level:   lv,
		Attrs:   ev.Attrs[:0],
	}

	for _, h := range l.Hooks {
		err := h(ctx, ev)
		if err != nil {
			return nil
		}
	}

	return
}

func (e *Event) Start() (s Span) {
	if e == nil {
		return Span{}
	}

	if e.Type == 0 {
		e.Type = 's'
	}

	s = Span{
		Logger: e.Logger,
		ID:     e.Span,
	}

	_ = e.Write()

	return
}

func (e *Event) Spawn(par ID) (s Span) {
	if e == nil {
		return Span{}
	}

	if e.Type == 0 {
		e.Type = 's'
	}

	e.Attrs = append(e.Attrs, A{Name: "p", Value: par})

	s = Span{
		Logger: e.Logger,
		ID:     e.Span,
	}

	_ = e.Write()

	return
}

func (e *Event) Finish() {
	if e == nil {
		return
	}

	if e.Type == 0 {
		e.Type = 'f'
	}

	_ = e.Write()
}

func (e *Event) Message(f string, args ...interface{}) {
	if e == nil {
		return
	}

	if f != "" {
		e.Fmt("m", f, args...)
	}

	_ = e.Write()
}

func (e *Event) Write() (err error) {
	if e == nil {
		return nil
	}

	err = e.Logger.Writer.Write(e)

	e.Reuse()

	return
}

func (e *Event) Reuse() {
	for _, a := range e.Attrs {
		if a.Value == nil {
			continue
		}
	}

	pool.Put(e)
}

func (e *Event) NewID() *Event {
	if e == nil {
		return nil
	}

	e.Span = e.Logger.NewID()

	return e
}

func (e *Event) Tp(t Type) *Event {
	if e == nil {
		return nil
	}

	e.Type = t

	return e
}

func (e *Event) Lv(lv Level) *Event {
	if e == nil {
		return nil
	}

	e.Level = lv

	return e
}

func (e *Event) Labels(ls Labels) *Event {
	if e == nil {
		return nil
	}

	o := (*slice)(unsafe.Pointer(&ls))

	s := poolslice.Get().(*slice)

	s.p = o.p
	s.l = o.l
	s.c = o.c

	sv := (*Labels)(unsafe.Pointer(s))

	e.Attrs = append(e.Attrs, A{Name: "L", Value: sv})

	return e
}

func (e *Event) Now() *Event {
	if e == nil {
		return nil
	}

	e.Time = now()

	return e
}

func (e *Event) Caller(d int) *Event {
	if e == nil {
		return nil
	}

	e.PC = Caller(1 + d)

	return e
}

func (e *Event) CallerOnce(d int, loc *PC) *Event {
	if e == nil {
		return nil
	}

	e.PC = PC(atomic.LoadUintptr((*uintptr)(unsafe.Pointer(loc))))

	if e.PC == 0 {
		e.PC = Caller(1 + d)
		atomic.StoreUintptr((*uintptr)(unsafe.Pointer(loc)), uintptr(e.PC))
	}

	return e
}

func (e *Event) Fmt(n, f string, args ...interface{}) *Event {
	if e == nil {
		return nil
	}

	st := len(e.b)
	e.b = AppendPrintf(e.b, f, args...)

	i := len(e.s)
	e.s = append(e.s, bytesToString(e.b[st:]))

	e.Attrs = append(e.Attrs, A{
		Name:  n,
		Value: &e.s[i],
	})

	return e
}

func (e *Event) Int(n string, v int) *Event {
	if e == nil {
		return nil
	}

	e.Attrs = append(e.Attrs, A{Name: n, Value: v})

	return e
}

func (e *Event) Str(n string, v string) *Event {
	if e == nil {
		return nil
	}

	e.Attrs = append(e.Attrs, A{Name: n, Value: v})

	return e
}

func (e *Event) Flt(n string, v float64) *Event {
	if e == nil {
		return nil
	}

	e.Attrs = append(e.Attrs, A{Name: n, Value: v})

	return e
}

func (e *Event) Any(n string, v interface{}) *Event {
	if e == nil {
		return nil
	}

	e.Attrs = append(e.Attrs, A{Name: n, Value: v})

	return e
}

func (e *Event) Append(a ...A) *Event {
	if e == nil {
		return nil
	}

	e.Attrs = append(e.Attrs, a...)

	return e
}

func (e *Event) Dict(d D) *Event {
	if e == nil {
		return nil
	}

	//	st := len(e.Attrs)

	for k, v := range d {
		e.Attrs = append(e.Attrs, A{Name: k, Value: v})
	}

	//	backup := e.Attrs

	//	e.Attrs = e.Attrs[st:]

	e.sorter = sorter(e.Attrs)

	sort.Sort(&e.sorter)

	//	fmt.Printf("sorted: %v\n", s)
	//	, func(i, j int) bool {
	//		return e.Attrs[st+i].Name < e.Attrs[st+j].Name
	//	})

	//	e.Attrs = backup

	return e
}

func (s *sorter) Len() int           { return len(*s) }
func (s *sorter) Less(i, j int) bool { return (*s)[i].Name < (*s)[j].Name }
func (s *sorter) Swap(i, j int)      { (*s)[i], (*s)[j] = (*s)[j], (*s)[i] }
