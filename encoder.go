package tlog

import (
	"fmt"
	"io"
	"math"
	"reflect"
	"time"

	"github.com/nikandfor/loc"
	"github.com/nikandfor/tlog/low"
)

type (
	Encoder struct {
		io.Writer

		lb Labels
		ls map[loc.PC]struct{}

		b []byte
	}

	Timestamp int64

	Format struct {
		Fmt  string
		Args []interface{}
	}
)

// basic types
const (
	Int = iota << 5
	Neg
	Bytes
	String
	Array
	Map
	Semantic
	Special

	TypeDetMask = 1<<5 - 1
	TypeMask    = 1<<8 - 1 - TypeDetMask
)

// len
const (
	_ = 1<<5 - iota
	LenBreak
	Len8
	Len4
	Len2
	Len1
)

// specials
const (
	False = 20 + iota
	True
	Null
	Undefined
	_
	Float16
	Float32
	Float64
	_
	_
	_
	Break
)

// semantic types
const (
	WTime = iota
	WDuration
	WError
	WID
	WLabels
	WLocation
)

func (e *Encoder) Encode(hdr []interface{}, kvs ...[]interface{}) (err error) {
	e.b = e.b[:0]

	e.b = append(e.b, Map|LenBreak)

	encodeKVs0(e, hdr...)

	for _, kvs := range kvs {
		encodeKVs0(e, kvs...)
	}

	e.b = append(e.b, Special|Break)

	_, err = e.Write(e.b)

	return err
}

func (e *Encoder) encodeKVs(kvs ...interface{}) {
	if len(kvs)&1 != 0 {
		panic("odd number of kvs")
	}

	for i, kv := range kvs {
		if lb, ok := kv.(Labels); ok && i > 0 {
			if k, ok := kvs[i].(string); ok && k == "L" {
				e.lb = lb
			}
		}

		e.appendValue(kv)
	}
}

func (e *Encoder) appendValue(v interface{}) {
	switch v := v.(type) {
	case nil:
		e.b = append(e.b, Special|Null)
	case string:
		e.b = appendString(e.b, String, v)
	case int:
		e.b = appendUint(e.b, Int, uint64(v))
	case float64:
		e.b = appendFloat(e.b, v)
	case Timestamp:
		e.b = append(e.b, Semantic|WTime)
		e.b = appendUint(e.b, Int, uint64(v))
	case time.Time:
		e.b = append(e.b, Semantic|WTime)
		e.b = appendUint(e.b, Int, uint64(v.UnixNano()))
	case time.Duration:
		e.b = append(e.b, Semantic|WDuration)
		e.b = appendUint(e.b, Int, uint64(v.Nanoseconds()))
	case ID:
		e.b = appendID(e.b, v)
	case loc.PC:
		e.appendPC(v)
	case Format:
		e.b = appendFormat(e.b, v)
	case error:
		e.b = append(e.b, Semantic|WError)
		e.b = appendString(e.b, String, v.Error())
	case fmt.Stringer:
		e.b = appendString(e.b, String, v.String())
	case []byte:
		e.b = appendString(e.b, Bytes, low.UnsafeBytesToString(v))
	default:
		r := reflect.ValueOf(v)
		e.appendRaw(r)
	}
}

func (e *Encoder) appendRaw(r reflect.Value) {
	switch r.Kind() {
	case reflect.String:
		e.b = appendString(e.b, String, r.String())
	case reflect.Int, reflect.Int64, reflect.Int32, reflect.Int16, reflect.Int8:
		e.b = appendInt(e.b, r.Int())
	case reflect.Uint, reflect.Uint64, reflect.Uint32, reflect.Uint16, reflect.Uint8:
		e.b = appendUint(e.b, Int, r.Uint())
	case reflect.Float64, reflect.Float32:
		e.b = appendFloat(e.b, r.Float())
	case reflect.Ptr, reflect.Interface:
		if r.IsNil() {
			e.b = append(e.b, Special|Null)
		} else {
			e.appendValue(r.Elem().Interface())
		}
	case reflect.Slice, reflect.Array:
		if r.Kind() == reflect.Slice && r.Type().Elem().Kind() == reflect.Uint8 {
			e.b = appendString(e.b, Bytes, low.UnsafeBytesToString(r.Bytes()))
			break
		}

		l := r.Len()

		e.b = appendTag(e.b, Array, l)

		for i := 0; i < l; i++ {
			e.appendValue(r.Index(i).Interface())
		}

	case reflect.Map:
		l := r.Len()

		e.b = appendTag(e.b, Map, l)

		it := r.MapRange()

		for it.Next() {
			e.appendValue(it.Key().Interface())
			e.appendValue(it.Value().Interface())
		}

	case reflect.Struct:
		e.appendStruct(r)

	default:
		panic(r)
	}
}

func (e *Encoder) appendStruct(r reflect.Value) {
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
		e.b = appendTag(e.b, Map, l)

		e.appendStructFields(t, r)

		return
	}

	e.b = append(e.b, Map|LenBreak)

	e.appendStructFields(t, r)

	e.b = append(e.b, Special|Break)

	return
}

func (e *Encoder) appendStructFields(t reflect.Type, r reflect.Value) {
	ff := t.NumField()

	for i := 0; i < ff; i++ {
		f := t.Field(i)

		if f.PkgPath != "" {
			continue
		}

		if f.Anonymous {
			e.appendStructFields(f.Type, r.Field(i))

			continue
		}

		e.b = appendString(e.b, String, f.Name)

		e.appendValue(r.Field(i).Interface())
	}
}

func (e *Encoder) appendPC(pc loc.PC) {
	if e.ls == nil {
		e.ls = make(map[loc.PC]struct{})
	}

	e.b = append(e.b, Semantic|WLocation)

	_, ok := e.ls[pc]
	if ok {
		e.b = appendUint(e.b, Int, uint64(pc))
		return
	}

	e.b = append(e.b, Map|4)

	e.b = appendString(e.b, String, "p")
	e.b = appendUint(e.b, Int, uint64(pc))

	name, file, line := pc.NameFileLine()

	e.b = appendString(e.b, String, "n")
	e.b = appendString(e.b, String, name)

	e.b = appendString(e.b, String, "f")
	e.b = appendString(e.b, String, file)

	e.b = appendString(e.b, String, "l")
	e.b = appendInt(e.b, int64(line))

	e.ls[pc] = struct{}{}
}

func appendID(b []byte, id ID) []byte {
	b = append(b, Semantic|WID)
	b = append(b, Bytes|16)
	b = append(b, id[:]...)
	return b
}

func appendString(b []byte, tag byte, s string) []byte {
	b = appendTag(b, tag, len(s))
	b = append(b, s...)

	return b
}

func appendFormat(b []byte, m Format) []byte {
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

func appendFloat(b []byte, v float64) []byte {
	if q := float32(v); float64(q) == v {
		r := math.Float32bits(q)

		return append(b, Special|Float32, byte(r>>24), byte(r>>16), byte(r>>8), byte(r))
	}

	r := math.Float64bits(v)

	return append(b, Special|Float64, byte(r>>56), byte(r>>48), byte(r>>40), byte(r>>32), byte(r>>24), byte(r>>16), byte(r>>8), byte(r))
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
	if v < 0 {
		return appendUint(b, Neg, uint64(-v))
	}

	return appendUint(b, Int, uint64(v))
}

func appendUint(b []byte, tag byte, v uint64) []byte {
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
