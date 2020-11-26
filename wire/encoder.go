package wire

import (
	"fmt"
	"io"
	"math"
	"reflect"
	"time"
	_ "unsafe"

	"github.com/nikandfor/tlog/core"
	"github.com/nikandfor/tlog/loc"
	"github.com/nikandfor/tlog/low"
)

// It's CBOR with some changes

type (
	Encoder struct {
		io.Writer

		ls map[loc.PC]struct{}

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

// Base types
const (
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

// Lengths
const (
	_ = 1<<5 - iota
	LenBreak
	Len8
	Len4
	Len2
	Len1
)

// Special values
const (
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

// Custom types
const (
	Time = iota
	Duration
	ID
	Location
	Labels

	Error
	_

	customEnd
)

// Record semantic tags
const (
	Type = customEnd + iota
	Level
	Parent

	Message
	Value
	UserFields
	_
	_

	_

	UserSemTagStart

	Name        = Message
	SpanElapsed = Duration
	Span        = ID
)

// Record types
const (
	Start      = 's'
	Finish     = 'f'
	MetricDesc = 'm'
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
	b := e.b[:0]

	b = append(b, Array|LenBreak)

	for _, t := range tags {
		switch t.T {
		case Time:
			b = append(b, Semantic|Time, Semantic|Time)
			b = appendInt(b, t.V.(int64))
		case Span:
			id := t.V.(core.ID)

			b = append(b, Semantic|Span, Semantic|ID, Bytes|byte(len(id)))
			b = append(b, id[:]...)
		case Message:
			b = append(b, Semantic|Message)

			switch v := t.V.(type) {
			case string:
				b = appendString(b, String, v)
			case Format:
				b = appendMessage(b, v)
			case []byte:
				b = appendString(b, String, low.UnsafeBytesToString(v))
			default:
				panic(v)
			}
		case Value:
			b = append(b, Semantic|Value)

			switch v := t.V.(type) {
			case int:
				b = appendInt(b, int64(v))
			case int64:
				b = appendInt(b, v)
			case float64:
				b = appendFloat(b, v)
			case float32:
				b = appendFloat(b, float64(v))
			default:
				b = e.appendValue(b, t.V)
			}
		case Type:
			var tp byte
			switch v := t.V.(type) {
			case rune:
				tp = byte(v)
			case byte:
				tp = byte(v)
			case int:
				tp = byte(v)
			case string:
				tp = v[0]
			default:
				panic(v)
			}

			b = append(b, Semantic|Type, String|1, tp)
		default:
			b = appendTag(b, Semantic, t.T)
			b = e.appendValue(b, t.V)
		}
	}

	if len(kvs) != 0 {
		b = append(b, Semantic|UserFields, Map|LenBreak)

		i := 0
		for i < len(kvs) {
			k := kvs[i]
			i++

			b = e.appendValue(b, k)

			v := kvs[i]
			i++

			b = e.appendValue(b, v)
		}

		b = append(b, Spec|Break)
	}

	b = append(b, Spec|Break)

	e.b = b

	_, _ = e.Writer.Write(b)
}

func (e *Encoder) appendValue(b []byte, v interface{}) (rb []byte) {
	//	defer func(st int) {
	//		fmt.Fprintf(os.Stderr, "append value % 2x <- %T %[2]v  | % x\n", rb[st:], v, b[:st])
	//	}(len(b))

	switch v := v.(type) {
	case nil:
		return append(b, Spec|Null)
	case string:
		return appendString(b, String, v)
	case core.ID:
		return e.appendID(b, v)
	case Format:
		return appendMessage(b, v)
	case error:
		b = append(b, Semantic|Error)
		return appendString(b, String, v.Error())
	case time.Duration:
		b = append(b, Semantic|Duration)
		return appendInt(b, v.Nanoseconds())
	case loc.PC:
		return e.appendPC(b, v)
	case fmt.Stringer:
		return appendString(b, String, v.String())
	}

	r := reflect.ValueOf(v)

	if (r.Kind() == reflect.Ptr || r.Kind() == reflect.Interface) && r.IsNil() {
		return append(b, Spec|Null)
	}

	if (r.Kind() == reflect.Slice || r.Kind() == reflect.Array) && r.Type().Elem().Kind() == reflect.Uint8 {
		if r.Kind() == reflect.Slice {
			return appendString(b, Bytes, low.UnsafeBytesToString(r.Bytes()))
		}

		l := r.Len()

		b = appendTag(b, Bytes, l)

		for i := 0; i < l; i++ {
			b = append(b, byte(r.Index(i).Uint()))
		}

		return b
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
	case reflect.Bool:
		if r.Bool() {
			return append(b, Spec|True)
		} else {
			return append(b, Spec|False)
		}
	case reflect.Slice, reflect.Array:
		l := r.Len()

		b = appendTag(b, Array, l)

		for i := 0; i < l; i++ {
			b = e.appendValue(b, r.Index(i).Interface())
		}

		return b
	case reflect.Map:
		l := r.Len()

		b = appendTag(b, Map, l)

		it := r.MapRange()

		for it.Next() {
			b = e.appendValue(b, it.Key().Interface())
			b = e.appendValue(b, it.Value().Interface())
		}

		return b
	case reflect.Struct:
		return e.appendStruct(b, v)
	case reflect.Ptr:
		return e.appendValue(b, r.Elem().Interface())
	default:
		panic(v)
	}
}

func (e *Encoder) appendStruct(b []byte, v interface{}) []byte {
	r := reflect.ValueOf(v)
	t := r.Type()
	ff := t.NumField()

	l := ff
	for i := 0; i < ff; i++ {
		f := t.Field(i)

		if f.Anonymous || f.PkgPath != "" {
			l = -1
			break
		}
	}

	if l != -1 {
		b = appendTag(b, Map, l)
		b = e.appendStructFields(b, t, r)

		return b
	}

	b = append(b, Map|LenBreak)

	b = e.appendStructFields(b, t, r)

	b = append(b, Spec|Break)

	return b
}

func (e *Encoder) appendStructFields(b []byte, t reflect.Type, r reflect.Value) []byte {
	ff := t.NumField()

	for i := 0; i < ff; i++ {
		f := t.Field(i)

		if f.PkgPath != "" {
			continue
		}

		if f.Anonymous {
			b = e.appendStructFields(b, f.Type, r.Field(i))
			continue
		}

		b = appendString(b, String, f.Name)

		b = e.appendValue(b, r.Field(i).Interface())
	}

	return b
}

func (e *Encoder) appendID(b []byte, id core.ID) []byte {
	b = append(b, Semantic|ID)
	b = append(b, Bytes|16)
	b = append(b, id[:]...)
	return b
}

func (e *Encoder) appendPC(b []byte, pc loc.PC) []byte {
	if e.ls == nil {
		e.ls = make(map[loc.PC]struct{})
	}

	b = append(b, Semantic|Location)

	_, ok := e.ls[pc]
	if ok {
		return appendUint(b, uint64(pc))
	}

	b = append(b, Map|4)

	b = appendString(b, String, "p")
	b = appendUint(b, uint64(pc))

	name, file, line := pc.NameFileLine()

	b = appendString(b, String, "n")
	b = appendString(b, String, name)

	b = appendString(b, String, "f")
	b = appendString(b, String, file)

	b = appendString(b, String, "l")
	b = appendInt(b, int64(line))

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

	if l < Len1 {
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
	case v < Len1:
		b = append(b, tag|byte(v))
	case v <= 0xff:
		b = append(b, tag|Len1, byte(v))
	case v <= 0xffff:
		b = append(b, tag|Len2, byte(v>>8), byte(v))
	case v <= 0xffff_ffff:
		b = append(b, tag|Len4, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	default:
		b = append(b, tag|Len8, byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32), byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	}

	b = append(b, s...)

	return b
}

func appendTag(b []byte, tag byte, v int) []byte {
	switch {
	case v < Len1:
		b = append(b, tag|byte(v))
	case v <= 0xff:
		b = append(b, tag|Len1, byte(v))
	case v <= 0xffff:
		b = append(b, tag|Len2, byte(v>>8), byte(v))
	case v <= 0xffff_ffff:
		b = append(b, tag|Len4, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	default:
		b = append(b, tag|Len8, byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32), byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
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
	case v < Len1:
		b = append(b, tag|byte(v))
	case v <= 0xff:
		b = append(b, tag|Len1, byte(v))
	case v <= 0xffff:
		b = append(b, tag|Len2, byte(v>>8), byte(v))
	case v <= 0xffff_ffff:
		b = append(b, tag|Len4, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	default:
		b = append(b, tag|Len8, byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32), byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	}

	return b
}

func appendUint(b []byte, v uint64) []byte {
	const tag = Int

	switch {
	case v < Len1:
		b = append(b, tag|byte(v))
	case v <= 0xff:
		b = append(b, tag|Len1, byte(v))
	case v <= 0xffff:
		b = append(b, tag|Len2, byte(v>>8), byte(v))
	case v <= 0xffff_ffff:
		b = append(b, tag|Len4, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	default:
		b = append(b, tag|Len8, byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32), byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
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
		b[st-1] |= Len1
		sz = 1
	case l <= 0xffff:
		b[st-1] |= Len2
		sz = 2
	case l <= 0xffff_ffff:
		b[st-1] |= Len4
		sz = 4
	default:
		b[st-1] |= Len8
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
