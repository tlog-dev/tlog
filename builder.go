package tlog

import (
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

type (
	MessageBuilder struct {
		*Logger
		Message
		Span ID
	}

	SpanStartBuilder struct {
		*Logger
		SpanStart
	}
)

var (
	msgPool  = sync.Pool{New: func() interface{} { return &MessageBuilder{} }}
	spanPool = sync.Pool{New: func() interface{} { return &SpanStartBuilder{} }}
)

func getMsgb(l *Logger, sid ID) (b *MessageBuilder) {
	b = msgPool.Get().(*MessageBuilder)

	b.Logger = l
	b.Span = sid

	b.Message = Message{Attrs: b.Message.Attrs[:0]}

	return b
}

func getSpanb(l *Logger) (b *SpanStartBuilder) {
	b = spanPool.Get().(*SpanStartBuilder)

	b.Logger = l

	b.SpanStart = SpanStart{}

	return b
}

func (l *Logger) BuildMessage() *MessageBuilder {
	if l == nil {
		return nil
	}

	return getMsgb(l, ID{})
}

func (s Span) BuildMessage() *MessageBuilder {
	if s.Logger == nil {
		return nil
	}

	return getMsgb(s.Logger, s.ID)
}

func (b *MessageBuilder) Caller(d int) *MessageBuilder {
	if b == nil {
		return nil
	}

	b.Message.PC = Caller(1 + d)

	return b
}

func (b *MessageBuilder) CallerOnce(d int, loc *PC) *MessageBuilder {
	if b == nil {
		return nil
	}

	l := (PC)(atomic.LoadUintptr((*uintptr)(unsafe.Pointer(loc))))
	if l == 0 {
		l = Caller(1 + d)
		atomic.StoreUintptr((*uintptr)(unsafe.Pointer(loc)), uintptr(l))
	}

	b.Message.PC = l

	return b
}

func (b *MessageBuilder) Location(pc PC) *MessageBuilder {
	if b == nil {
		return nil
	}

	b.Message.PC = pc

	return b
}

func (b *MessageBuilder) Now() *MessageBuilder {
	if b == nil {
		return nil
	}

	b.Message.Time = now()

	return b
}

func (b *MessageBuilder) Time(t time.Time) *MessageBuilder {
	if b == nil {
		return nil
	}

	b.Message.Time = t

	return b
}

func (b *MessageBuilder) Level(l Level) *MessageBuilder {
	if b == nil {
		return nil
	}

	b.Message.Level = l

	return b
}

func (b *MessageBuilder) SpanID(sid ID) *MessageBuilder {
	if b == nil {
		return nil
	}

	b.Span = sid

	return b
}

func (b *MessageBuilder) Int(n string, v int) *MessageBuilder {
	if b == nil {
		return nil
	}

	b.Message.Attrs = append(b.Message.Attrs, Attr{Name: n, Value: v})

	return b
}

func (b *MessageBuilder) Int64(n string, v int64) *MessageBuilder {
	if b == nil {
		return nil
	}

	b.Message.Attrs = append(b.Message.Attrs, Attr{Name: n, Value: v})

	return b
}

func (b *MessageBuilder) Uint64(n string, v int64) *MessageBuilder {
	if b == nil {
		return nil
	}

	b.Message.Attrs = append(b.Message.Attrs, Attr{Name: n, Value: v})

	return b
}

func (b *MessageBuilder) Str(n, v string) *MessageBuilder {
	if b == nil {
		return nil
	}

	b.Message.Attrs = append(b.Message.Attrs, Attr{Name: n, Value: v})

	return b
}

func (b *MessageBuilder) ID(n string, v ID) *MessageBuilder {
	if b == nil {
		return nil
	}

	b.Message.Attrs = append(b.Message.Attrs, Attr{Name: n, Value: v})

	return b
}

func (b *MessageBuilder) Printf(f string, args ...interface{}) {
	if b == nil {
		return
	}

	if len(args) == 0 {
		b.Message.Text = f
	} else {
		buf, wr := Getbuf()
		defer wr.Ret(&buf)

		if f != "" {
			buf = AppendPrintf(buf, f, args...)
		} else {
			buf = AppendPrintln(buf, args...)
		}

		b.Message.Text = bytesToString(buf)
	}

	_ = b.Logger.Writer.Message(b.Message, b.Span)

	msgPool.Put(b)
}

func (l *Logger) BuildSpanStart() *SpanStartBuilder {
	if l == nil {
		return nil
	}

	return getSpanb(l)
}

func (s Span) BuildSpanStart() (b *SpanStartBuilder) {
	if s.Logger == nil {
		return nil
	}

	b = getSpanb(s.Logger)
	b.SpanStart.Parent = s.ID

	return b
}

func (b *SpanStartBuilder) Caller(d int) *SpanStartBuilder {
	if b == nil {
		return nil
	}

	b.SpanStart.PC = Caller(1 + d)

	return b
}

func (b *SpanStartBuilder) CallerOnce(d int, loc *PC) *SpanStartBuilder {
	if b == nil {
		return nil
	}

	l := (PC)(atomic.LoadUintptr((*uintptr)(unsafe.Pointer(loc))))
	if l == 0 {
		l = Caller(1 + d)
		atomic.StoreUintptr((*uintptr)(unsafe.Pointer(loc)), uintptr(l))
	}

	b.SpanStart.PC = l

	return b
}

func (b *SpanStartBuilder) Location(pc PC) *SpanStartBuilder {
	if b == nil {
		return nil
	}

	b.SpanStart.PC = pc

	return b
}

func (b *SpanStartBuilder) Now() *SpanStartBuilder {
	if b == nil {
		return nil
	}

	b.SpanStart.StartedAt = now()

	return b
}

func (b *SpanStartBuilder) Time(t time.Time) *SpanStartBuilder {
	if b == nil {
		return nil
	}

	b.SpanStart.StartedAt = t

	return b
}

func (b *SpanStartBuilder) Parent(id ID) *SpanStartBuilder {
	if b == nil {
		return nil
	}

	b.SpanStart.Parent = id

	return b
}

func (b *SpanStartBuilder) NewID() *SpanStartBuilder {
	if b == nil {
		return nil
	}

	b.SpanStart.ID = b.Logger.NewID()

	return b
}

func (b *SpanStartBuilder) ID(id ID) *SpanStartBuilder {
	if b == nil {
		return nil
	}

	b.SpanStart.ID = id

	return b
}

func (b *SpanStartBuilder) Start() (s Span) {
	if b == nil {
		return Span{}
	}

	if b.SpanStart.ID == (ID{}) {
		panic("zero id")
	}

	_ = b.Logger.Writer.SpanStarted(b.SpanStart)

	s = Span{Logger: b.Logger, ID: b.SpanStart.ID, StartedAt: b.SpanStart.StartedAt}

	spanPool.Put(b)

	return s
}
