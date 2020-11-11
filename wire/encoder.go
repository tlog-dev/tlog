package wire

import (
	"io"
	"math"
	"reflect"
	_ "unsafe"

	"github.com/nikandfor/tlog/low"
	"github.com/nikandfor/tlog/tlt"
)

// It's CBOR with some changes

type (
	Encoder struct {
		io.Writer

		b []byte
	}

	Tag struct {
		T int
		V interface{}
	}

	Format struct {
		Fmt  string
		Args []interface{}
	}
)

const ( // record semantic types
	EOR = iota
	Time
	Type
	Level
	Labels

	Location
	Span
	Parent
	Duration
	Message

	Value
	Fields
	Func
	File
	Line

	ID   = Span
	Name = Message

	UserTagStart = 18
)

const ( // record types
	Start  = 's'
	Finish = 'f'
)

const ( // base types
	Int = iota << 5
	Neg
	Bytes
	String
	Array
	Map
	Semantic
	Spec

	TypeMask    = 0b1110_0000
	TypeDetMask = 0b0001_1111
)

const ( // specials
	False = 20 + iota
	True
	Null
	Undefined
	_
	Float16
	Float32
	Float64

	Break = 31
)

//go:linkname AppendTagVal github.com/nikandfor/tlog/wire.appendTagVal
//go:noescape
func AppendTagVal(tags []Tag, t int, v interface{}) []Tag

func appendTagVal(tags []Tag, t int, v interface{}) []Tag {
	return append(tags, Tag{T: t, V: v})
}

//go:linkname Event github.com/nikandfor/tlog/wire.event
//go:noescape
func Event(e *Encoder, tags []Tag, kvs []interface{})

func event(e *Encoder, tags []Tag, kvs []interface{}) {
	defer func() {
		// NOTE: since we hacked compiler and made all arguments not escaping
		// we must zero all possible pointers to stack

		for i := range tags {
			tags[i].V = nil
		}
	}()

	b := e.b[:0]

	for _, t := range tags {
		b = appendTag(b, Semantic, t.T)
		b = appendValue(b, t.V)
	}

	if len(kvs) != 0 {
		b = append(b, Map)

		i := 0
		for i < len(kvs) {
			k := kvs[i]
			i++

			b = appendTypedValue(b, k)

			v := kvs[i]
			i++

			b = appendTypedValue(b, v)
		}

		b = append(b, Spec|Break)
	}

	b = append(b, Semantic|EOR)

	e.b = b

	_, _ = e.Writer.Write(b)
}

func appendTypedValue(b []byte, v interface{}) []byte {
	switch v := v.(type) {
	case tlt.ID:
		return appendID(b, v, true)
	}

	return appendValue(b, v)
}

func appendValue(b []byte, v interface{}) (rb []byte) {
	//	defer func(st int) {
	//		fmt.Fprintf(os.Stderr, "append value % 2x | % 2x <- %T %[3]v\n", b[:st], rb[st:], v)
	//	}(len(b))

	switch v := v.(type) {
	case nil:
		panic("nil value")
	case string:
		return appendString(b, String, v)
	case tlt.ID:
		return appendID(b, v, false)
	case Format:
		return appendMessage(b, v)
	}

	r := reflect.ValueOf(v)

	if r.Kind() == reflect.Slice && r.Type().Elem().Kind() == reflect.Uint8 {
		return appendString(b, Bytes, low.UnsafeBytesToString(r.Bytes()))
	}

	switch r.Kind() {
	case reflect.String:
		return appendString(b, String, r.String())
	case reflect.Int, reflect.Int64, reflect.Int32, reflect.Int16, reflect.Int8:
		return appendInt(b, r.Int())
	case reflect.Uint, reflect.Uint64, reflect.Uint32, reflect.Uint16, reflect.Uint8:
		return appendUint(b, r.Uint())
	case reflect.Float64, reflect.Float32:
		return appendFloat(b, r.Float())
	case reflect.Slice, reflect.Array:
		l := r.Len()

		b = appendTag(b, Array, l)

		for i := 0; i < l; i++ {
			b = appendValue(b, r.Index(i).Interface())
		}

		return b
	default:
		panic(v)
	}
}

func appendID(b []byte, id tlt.ID, typed bool) []byte {
	if typed {
		b = append(b, Semantic|ID)
	}
	b = append(b, Bytes|16)
	b = append(b, id[:]...)
	return b
}

func appendMessage(b []byte, m Format) []byte {
	b = append(b, String)

	st := len(b)

	switch {
	case len(m.Args) == 0:
		b = append(b, m.Fmt...)
	case m.Fmt == "":
		b = low.AppendPrintln(b, m.Args...)
	default:
		b = low.AppendPrintf(b, m.Fmt, m.Args...)
	}

	l := len(b) - st

	if l < 1<<5-4 {
		b[st-1] |= byte(l)

		return b
	}

	//	fmt.Fprintf(os.Stderr, "msg before % 2x\n", b[st-1:])

	b = insertLen(b, st, l)

	//	fmt.Fprintf(os.Stderr, "msg after  % 2x\n", b[st-1:])

	return b
}

func appendString(b []byte, tag byte, s string) []byte {
	v := len(s)

	switch {
	case v < 1<<5-4:
		b = append(b, tag|byte(v))
	case v <= 0xff:
		b = append(b, tag|1<<5-4, byte(v))
	case v <= 0xffff:
		b = append(b, tag|1<<5-3, byte(v>>8), byte(v))
	case v <= 0xffff_ffff:
		b = append(b, tag|1<<5-2, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	default:
		b = append(b, tag|1<<5-1, byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32), byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	}

	b = append(b, s...)

	return b
}

func appendTag(b []byte, tag byte, v int) []byte {
	switch {
	case v < 1<<5-4:
		b = append(b, tag|byte(v))
	case v <= 0xff:
		b = append(b, tag|1<<5-4, byte(v))
	case v <= 0xffff:
		b = append(b, tag|1<<5-3, byte(v>>8), byte(v))
	case v <= 0xffff_ffff:
		b = append(b, tag|1<<5-2, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	default:
		b = append(b, tag|1<<5-1, byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32), byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	}

	return b
}

func appendInt(b []byte, v int64) []byte {
	var tag byte
	if v >= 0 {
		tag = Int
	} else {
		tag = Neg
		v = -v + 1
	}

	switch {
	case v < 1<<5-4:
		b = append(b, tag|byte(v))
	case v <= 0xff:
		b = append(b, tag|1<<5-4, byte(v))
	case v <= 0xffff:
		b = append(b, tag|1<<5-3, byte(v>>8), byte(v))
	case v <= 0xffff_ffff:
		b = append(b, tag|1<<5-2, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	default:
		b = append(b, tag|1<<5-1, byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32), byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	}

	return b
}

func appendUint(b []byte, v uint64) []byte {
	const tag = Int

	switch {
	case v < 1<<5-4:
		b = append(b, tag|byte(v))
	case v <= 0xff:
		b = append(b, tag|1<<5-4, byte(v))
	case v <= 0xffff:
		b = append(b, tag|1<<5-3, byte(v>>8), byte(v))
	case v <= 0xffff_ffff:
		b = append(b, tag|1<<5-2, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	default:
		b = append(b, tag|1<<5-1, byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32), byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	}

	return b
}

func appendFloat(b []byte, v float64) []byte {
	if q := float32(v); float64(q) == v {
		r := math.Float32bits(q)

		return append(b, Spec|Float32, byte(r>>24), byte(r>>16), byte(r>>8), byte(r))
	}

	r := math.Float64bits(v)

	return append(b, Spec|Float64, byte(r>>56), byte(r>>48), byte(r>>40), byte(r>>32), byte(r>>24), byte(r>>16), byte(r>>8), byte(r))
}

func insertLen(b []byte, st, l int) []byte {
	var sz int

	switch {
	case l <= 0xff:
		b[st-1] |= 1<<5 - 4
		sz = 1
	case l <= 0xffff:
		b[st-1] |= 1<<5 - 3
		sz = 2
	case l <= 0xffff_ffff:
		b[st-1] |= 1<<5 - 2
		sz = 4
	default:
		b[st-1] |= 1<<5 - 1
		sz = 8
	}

	b = append(b, []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0}[:sz]...)
	copy(b[st+sz:], b[st:])

	for i := st + sz - 1; i >= st; i-- {
		b[i] = byte(l)
		l >>= 8
	}

	return b
}
