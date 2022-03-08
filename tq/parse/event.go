package parse

import (
	"context"
	"sync"

	"github.com/nikandfor/errors"
	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/wire"
)

type (
	Event struct {
		Timestamp int64

		Spans []tlog.ID

		KVs []LazyKV

		Labels Labels

		raw []byte `deep:"-"`

		spansbuf [1]tlog.ID `deep:"-"`
		kvbuf    [2]LazyKV  `deep:"-"`
	}

	LazyKV struct {
		K String
		V []byte
	}

	Labels []byte

	LowParser struct {
		New func() *LowEvent
	}

	LowEvent struct {
		ts int64
		ls []byte

		kv []int

		raw []byte

		kvbuf [6]int
	}
)

var lowEvents = sync.Pool{
	New: func() interface{} {
		return new(LowEvent)
	},
}

func NewLowEvent() (e *LowEvent) {
	return lowEvents.Get().(*LowEvent)
}

func FreeLowEvent(e *LowEvent) {
	e.Reset()

	lowEvents.Put(e)
}

func (n *LowParser) Parse(ctx context.Context, p []byte, st int) (x interface{}, i int, err error) {
	var d wire.Decoder

	e := n.New()

	tag, els, i := d.Tag(p, st)
	if tag != wire.Map {
		return nil, st, errors.New("Event expected")
	}

	e.kv = append(e.kv, i)

	var k []byte
	var sub int64
	for el := 0; els == -1 || el < int(els); el++ {
		if els == -1 && d.Break(p, &i) {
			break
		}

		// key
		k, i = d.String(p, i)

		// value
		st := i
		tag, sub, i = d.SkipTag(p, i)

		e.kv = append(e.kv, i) // end of pair

		if tag != wire.Semantic {
			continue
		}

		switch {
		case sub == wire.Time && string(k) == tlog.KeyTime:
			e.ts, _ = d.Timestamp(p, st)
		case sub == tlog.WireLabels && string(k) == tlog.KeyLabels:
			if e.ls == nil {
				e.ls = p[st:i:i]
				break
			}

			e.ls = append(e.ls, p[st:i]...)
		}
	}

	e.raw = p

	return e, i, nil
}

func (e *LowEvent) Reset() {
	e.ts = 0
	e.raw = nil

	if e.kv != nil {
		e.kv = e.kv[:0]
	} else {
		e.kv = e.kvbuf[:0]
	}
}

func (e *LowEvent) Bytes() []byte {
	return e.raw
}

func (e *LowEvent) Timestamp() int64 {
	return e.ts
}

func (e *LowEvent) Labels() []byte {
	return e.ls
}

func (e *LowEvent) Len() int {
	if len(e.kv) == 0 {
		return 0
	}

	return len(e.kv) - 1
}

func (e *LowEvent) Index(i int) (keyContent, rawValue []byte) {
	var d wire.LowDecoder

	off := e.kv[i]

	keyContent, off = d.String(e.raw, off)
	rawValue = e.raw[off:e.kv[i+1]]

	return
}

func (e *LowEvent) Slice(st, end int) []byte {
	return e.raw[e.kv[st]:e.kv[end]]
}

func (e *Event) Parse(ctx context.Context, p []byte, st int) (x interface{}, i int, err error) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}

		err = errors.NewDepth(2, "panic: %v", r)
	}()

	e.Reset()

	var d wire.Decoder

	tag, els, i := d.Tag(p, st)
	if tag != wire.Map {
		return nil, st, errors.New("Event expected")
	}

	var k []byte
	var sub int64
	for el := 0; els == -1 || el < int(els); el++ {
		if els == -1 && d.Break(p, &i) {
			break
		}

		st := i

		k, i = d.String(p, i)

		tag, sub, _ = d.Tag(p, i)
		if tag != wire.Semantic {
			i, err = e.parseKV(ctx, p, k, st, i)
			if err != nil {
				return nil, i, errors.Wrap(err, "%s", k)
			}

			continue
		}

		switch {
		case sub == wire.Time && string(k) == tlog.KeyTime:
			e.Timestamp, i = d.Timestamp(p, i)
		case sub == tlog.WireID && string(k) == tlog.KeySpan:
			var id tlog.ID
			i = id.TlogParse(&d, p, i)

			e.Spans = append(e.Spans, id)
		case sub == tlog.WireLabels && string(k) == tlog.KeyLabels:
			vst := i
			i = d.Skip(p, i)

			e.Labels = p[vst:i]
		default:
			i, err = e.parseKV(ctx, p, k, st, i)
		}

		if err != nil {
			return nil, i, errors.Wrap(err, "%s", k)
		}
	}

	e.raw = p

	return e, i, nil
}

func (e *Event) parseKV(ctx context.Context, p, k []byte, st, vst int) (i int, err error) {
	var d wire.LowDecoder

	i = d.Skip(p, vst)

	kv := LazyKV{
		K: String(k),
		V: p[vst:i],
	}

	e.KVs = append(e.KVs, kv)

	return i, nil
}

func (e *Event) Reset() {
	e.Timestamp = 0
	e.Labels = nil
	e.raw = nil

	for i := range e.KVs {
		e.KVs[i] = LazyKV{}
	}

	if len(e.Spans) > 0 {
		e.Spans = e.Spans[:0]
	} else {
		e.Spans = e.spansbuf[:0]
	}

	if len(e.KVs) > 0 {
		e.KVs = e.KVs[:0]
	} else {
		e.KVs = e.kvbuf[:0]
	}
}
