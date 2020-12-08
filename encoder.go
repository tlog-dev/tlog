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

		Labels Labels
		ls     map[loc.PC]struct{}

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
	WireTime = iota
	WireDuration
	WireError
	WireID
	WireLabels
	WireLocation
)

func (e *Encoder) Encode(hdr []interface{}, kvs ...[]interface{}) (err error) {
	//	old := e.Labels

	if e.ls == nil {
		e.ls = make(map[loc.PC]struct{})
	}

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

	for i := 0; i < len(kvs); {
		e.b = e.AppendString(e.b, String, kvs[i].(string))
		i++

		e.b = e.AppendValue(e.b, kvs[i])
		i++
	}
}

func (e *Encoder) AppendValue(b []byte, v interface{}) []byte {
	switch v := v.(type) {
	case nil:
		return append(b, Special|Null)
	case string:
		return e.AppendString(b, String, v)
	case int:
		return e.AppendUint(b, Int, uint64(v))
	case float64:
		return e.AppendFloat(b, v)
	case Timestamp:
		b = append(b, Semantic|WireTime)
		return e.AppendUint(b, Int, uint64(v))
	case time.Time:
		b = append(b, Semantic|WireTime)
		return e.AppendUint(b, Int, uint64(v.UnixNano()))
	case time.Duration:
		b = append(b, Semantic|WireDuration)
		return e.AppendUint(b, Int, uint64(v.Nanoseconds()))
	case ID:
		return e.AppendID(b, v)
	case loc.PC:
		return e.AppendPC(b, v, true)
	case Format:
		return e.AppendFormat(b, v)
	case Labels:
		return e.AppendLabels(b, v)
	case error:
		b = append(b, Semantic|WireError)
		return e.AppendString(b, String, v.Error())
	case fmt.Stringer:
		return e.AppendString(b, String, v.String())
	case []byte:
		return e.AppendString(b, Bytes, low.UnsafeBytesToString(v))
	default:
		r := reflect.ValueOf(v)
		return e.appendRaw(b, r)
	}
}

func (e *Encoder) appendRaw(b []byte, r reflect.Value) []byte {
	switch r.Kind() {
	case reflect.String:
		return e.AppendString(b, String, r.String())
	case reflect.Int, reflect.Int64, reflect.Int32, reflect.Int16, reflect.Int8:
		return e.AppendInt(b, r.Int())
	case reflect.Uint, reflect.Uint64, reflect.Uint32, reflect.Uint16, reflect.Uint8:
		return e.AppendUint(b, Int, r.Uint())
	case reflect.Float64, reflect.Float32:
		return e.AppendFloat(b, r.Float())
	case reflect.Ptr, reflect.Interface:
		if r.IsNil() {
			return append(b, Special|Null)
		} else {
			return e.AppendValue(b, r.Elem().Interface())
		}
	case reflect.Slice, reflect.Array:
		if r.Kind() == reflect.Slice && r.Type().Elem().Kind() == reflect.Uint8 {
			return e.AppendString(b, Bytes, low.UnsafeBytesToString(r.Bytes()))
		}

		l := r.Len()

		b = e.AppendTag(b, Array, l)

		for i := 0; i < l; i++ {
			b = e.AppendValue(b, r.Index(i).Interface())
		}

		return b
	case reflect.Map:
		l := r.Len()

		b = e.AppendTag(b, Map, l)

		it := r.MapRange()

		for it.Next() {
			b = e.AppendValue(b, it.Key().Interface())
			b = e.AppendValue(b, it.Value().Interface())
		}

		return b
	case reflect.Struct:
		return e.appendStruct(b, r)
	default:
		panic(r)
	}
}

func (e *Encoder) appendStruct(b []byte, r reflect.Value) []byte {
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
		b = e.AppendTag(b, Map, l)

		return e.appendStructFields(b, t, r)
	}

	b = append(b, Map|LenBreak)

	b = e.appendStructFields(b, t, r)

	b = append(b, Special|Break)

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

		b = e.AppendString(b, String, f.Name)

		b = e.AppendValue(b, r.Field(i).Interface())
	}

	return b
}

func (e *Encoder) AppendPC(b []byte, pc loc.PC, cache bool) []byte {
	b = append(b, Semantic|WireLocation)

	if e == nil || e.ls == nil || !cache {
		return e.AppendUint(b, Int, uint64(pc))
	}

	if _, ok := e.ls[pc]; ok {
		return e.AppendUint(b, Int, uint64(pc))
	}

	b = append(b, Map|4)

	b = e.AppendString(b, String, "p")
	b = e.AppendUint(b, Int, uint64(pc))

	name, file, line := pc.NameFileLine()

	b = e.AppendString(b, String, "n")
	b = e.AppendString(b, String, name)

	b = e.AppendString(b, String, "f")
	b = e.AppendString(b, String, file)

	b = e.AppendString(b, String, "l")
	b = e.AppendInt(b, int64(line))

	e.ls[pc] = struct{}{}

	return b
}

func (e *Encoder) AppendLabels(b []byte, ls Labels) []byte {
	b = append(b, Semantic|WireLabels)
	b = e.AppendTag(b, Array, len(ls))

	for _, l := range ls {
		b = e.AppendString(b, String, l)
	}

	return b
}

func (_ *Encoder) AppendID(b []byte, id ID) []byte {
	b = append(b, Semantic|WireID)
	b = append(b, Bytes|16)
	b = append(b, id[:]...)

	return b
}

func (e *Encoder) AppendString(b []byte, tag byte, s string) []byte {
	b = e.AppendTag(b, tag, len(s))
	return append(b, s...)
}

func (e *Encoder) AppendFormat(b []byte, m Format) []byte {
	if len(m.Args) == 0 {
		return e.AppendString(b, String, m.Fmt)
	}

	b = append(b, String)

	st := len(b)

	if m.Fmt == "" {
		b = low.AppendPrintln(b, m.Args...)
	} else {
		b = low.AppendPrintf(b, m.Fmt, m.Args...)
	}

	l := len(b) - st

	if l < Len1 {
		b[st-1] |= byte(l)

		return b
	}

	//	fmt.Fprintf(os.Stderr, "msg before % 2x\n", b[st-1:])

	b = e.insertLen(b, st, l)

	//	fmt.Fprintf(os.Stderr, "msg after  % 2x\n", b[st-1:])

	return b
}

func (_ *Encoder) insertLen(b []byte, st, l int) []byte {
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

func (_ *Encoder) AppendFloat(b []byte, v float64) []byte {
	if q := float32(v); float64(q) == v {
		r := math.Float32bits(q)

		return append(b, Special|Float32, byte(r>>24), byte(r>>16), byte(r>>8), byte(r))
	}

	r := math.Float64bits(v)

	return append(b, Special|Float64, byte(r>>56), byte(r>>48), byte(r>>40), byte(r>>32), byte(r>>24), byte(r>>16), byte(r>>8), byte(r))
}

func (_ *Encoder) AppendTag(b []byte, tag byte, v int) []byte {
	switch {
	case v < Len1:
		return append(b, tag|byte(v))
	case v <= 0xff:
		return append(b, tag|Len1, byte(v))
	case v <= 0xffff:
		return append(b, tag|Len2, byte(v>>8), byte(v))
	case v <= 0xffff_ffff:
		return append(b, tag|Len4, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	default:
		return append(b, tag|Len8, byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32), byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	}
}

func (e *Encoder) AppendInt(b []byte, v int64) []byte {
	if v < 0 {
		return e.AppendUint(b, Neg, uint64(-v))
	}

	return e.AppendUint(b, Int, uint64(v))
}

func (_ *Encoder) AppendUint(b []byte, tag byte, v uint64) []byte {
	switch {
	case v < Len1:
		return append(b, tag|byte(v))
	case v <= 0xff:
		return append(b, tag|Len1, byte(v))
	case v <= 0xffff:
		return append(b, tag|Len2, byte(v>>8), byte(v))
	case v <= 0xffff_ffff:
		return append(b, tag|Len4, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	default:
		return append(b, tag|Len8, byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32), byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	}
}