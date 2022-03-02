package parse

import (
	"context"

	"github.com/nikandfor/errors"
	"github.com/nikandfor/tlog/wire"
)

type (
	Value struct{}

	Lazy struct {
		c interface{}
		r []byte
	}

	Int []byte

	Bytes []byte

	String []byte

	Array []interface{}

	Map []KV

	KV struct {
		K String
		V []byte
	}

	Semantic struct {
		Code int
		V    interface{}
	}

	Nil struct{}

	Undefined struct{}

	Float []byte
)

func (Value) Parse(ctx context.Context, p []byte, st int) (x interface{}, i int, err error) {
	var d wire.LowDecoder

	tag := d.TagOnly(p, st)

	switch tag {
	case wire.Int, wire.Neg:
		return Int{}.Parse(ctx, p, st)
	case wire.Bytes:
		return Bytes{}.Parse(ctx, p, st)
	case wire.String:
		return String{}.Parse(ctx, p, st)
	case wire.Array:
		return Array{}.Parse(ctx, p, st)
	case wire.Map:
		return Map{}.Parse(ctx, p, st)
	case wire.Semantic:
		var sub int64
		tag, sub, i = d.Tag(p, st)

		var v interface{}
		v, i, err = Value{}.Parse(ctx, p, i)
		if err != nil {
			return
		}

		return Semantic{
			Code: int(sub),
			V:    v,
		}, i, nil
	case wire.Special:
		var sub int64
		tag, sub, i = d.Tag(p, st)

		switch sub {
		case wire.False:
			return false, i, nil
		case wire.True:
			return true, i, nil
		case wire.Nil:
			return Nil{}, i, nil
		case wire.Undefined:
			return Undefined{}, i, nil
		case wire.Float8, wire.Float16, wire.Float32, wire.Float64:
			i += 1 << (sub - wire.Float8)

			return Float(p[st:i:i]), i, nil
		default:
			panic(sub)
		}
	default:
		panic(tag)
	}
}

func (n Int) Parse(ctx context.Context, p []byte, st int) (x interface{}, i int, err error) {
	var d wire.LowDecoder

	tag, _, i := d.Tag(p, st)
	if tag != wire.Int && tag != wire.Neg {
		return nil, st, errors.New("Int expected")
	}

	return Int(p[st:i:i]), i, nil
}

func (n Bytes) Parse(ctx context.Context, p []byte, st int) (x interface{}, i int, err error) {
	var d wire.LowDecoder

	tag, l, i := d.Tag(p, st)
	if tag != wire.Bytes {
		return nil, st, errors.New("Bytes expected")
	}

	st = i
	i += int(l)

	return Bytes(p[st:i:i]), i, nil
}

func (n String) Parse(ctx context.Context, p []byte, st int) (x interface{}, i int, err error) {
	var d wire.LowDecoder

	tag, l, i := d.Tag(p, st)
	if tag != wire.String {
		return nil, st, errors.New("String expected")
	}

	st = i
	i += int(l)

	return String(p[st:i:i]), i, nil
}

func (n String) TlogAppend(e *wire.Encoder, b []byte) []byte {
	return e.AppendTagBytes(b, wire.String, n)
}

func (n Array) Parse(ctx context.Context, p []byte, st int) (x interface{}, i int, err error) {
	var d wire.LowDecoder

	tag, els, i := d.Tag(p, st)
	if tag != wire.Array {
		return nil, st, errors.New("Array expected")
	}

	var v interface{}

	for el := 0; els == -1 || el < int(els); el++ {
		if els == -1 && d.Break(p, &i) {
			break
		}

		v, i, err = Value{}.Parse(ctx, p, i)
		if err != nil {
			return nil, i, err
		}

		n = append(n, v)
	}

	return n, i, nil
}

func (n Map) Parse(ctx context.Context, p []byte, st int) (x interface{}, i int, err error) {
	var d wire.LowDecoder

	tag, els, i := d.Tag(p, st)
	if tag != wire.Map {
		return nil, st, errors.New("Map expected")
	}

	var k []byte

	for el := 0; els == -1 || el < int(els); el++ {
		if els == -1 && d.Break(p, &i) {
			break
		}

		k, i = d.String(p, i)

		vst := i
		i = d.Skip(p, i)

		n = append(n, KV{K: k, V: p[vst:i]})
	}

	return n, i, nil
}
