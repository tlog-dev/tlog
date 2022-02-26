package parse

import (
	"context"

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

		raw []byte

		spansbuf [1]tlog.ID
		kvbuf    [2]LazyKV
	}

	LazyKV struct {
		K String

		v interface{}
		r []byte
	}

	Labels []byte
)

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
		r: p[vst:i],
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
