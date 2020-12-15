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
		pos int

		Labels Labels
		ls     map[loc.PC]struct{}

		b []byte
	}

	Message   string
	Name      string
	LogLevel  int
	Timestamp int64
	Hex       int64

	FormatNext string

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
	FloatInt8
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
	WireMeta = iota
	WireTime
	WireDuration
	WireMessage
	WireName
	WireError
	WireID
	WireLabels
	WireLocation
	WireHex
	WireLogLevel
)

func (e *Encoder) Encode(hdr []interface{}, kvs ...[]interface{}) (err error) {
	if e.ls == nil {
		e.ls = make(map[loc.PC]struct{})
	}

	//	old := e.Labels

	e.b = e.b[:0]

	l := e.calcMapLen(hdr)
	for _, kvs := range kvs {
		l += e.calcMapLen(kvs)
	}

	if l == 0 {
		return nil
	}

	e.b = e.AppendTag(e.b, Map, l)

	if len(hdr) != 0 {
		encodeKVs0(e, hdr...)
	}

	for _, kvs := range kvs {
		if len(kvs) != 0 {
			encodeKVs0(e, kvs...)
		}
	}

	l, err = e.Write(e.b)
	e.pos += l

	return err
}

func (e *Encoder) calcMapLen(kvs []interface{}) (l int) {
	for i := 0; i < len(kvs); i++ {
		if _, ok := kvs[i].(string); !ok {
			panic("key must be string")
		}

		if i == len(kvs) {
			panic("no value for key")
		}

		i++

		if _, ok := kvs[i].(FormatNext); ok {
			if i == len(kvs) {
				panic("no argument for FormatNext")
			}
			i++
		}

		l++
	}

	return
}

func (e *Encoder) encodeKVs(kvs ...interface{}) {
	for i := 0; i < len(kvs); {
		e.b = e.AppendString(e.b, String, kvs[i].(string))
		i++

		switch v := kvs[i].(type) {
		case FormatNext:
			e.b = e.AppendFormat(e.b, string(v), kvs[i+1])
			i += 2
		default:
			e.b = e.AppendValue(e.b, kvs[i])
			i++
		}
	}
}

func (e *Encoder) AppendValue(b []byte, v interface{}) []byte {
	switch v := v.(type) {
	case nil:
		return append(b, Special|Null)
	case Message:
		b = append(b, Semantic|WireMessage)
		return e.AppendString(b, String, string(v))
	case Name:
		b = append(b, Semantic|WireName)
		return e.AppendString(b, String, string(v))
	case string:
		return e.AppendString(b, String, v)
	case int:
		return e.AppendInt(b, int64(v))
	case float64:
		return e.AppendFloat(b, v)
	case Hex:
		b = append(b, Semantic|WireHex)
		return e.AppendInt(b, int64(v))
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
		return e.AppendLoc(b, v, true)
	case Format:
		return e.AppendFormat(b, v.Fmt, v.Args...)
	case Labels:
		return e.AppendLabels(b, v)
	case LogLevel:
		b = append(b, Semantic|WireLogLevel)
		return e.AppendUint(b, Int, uint64(v))
	case error:
		b = append(b, Semantic|WireError)
		return e.AppendString(b, String, v.Error())
	case fmt.Stringer:
		return e.AppendString(b, String, v.String())
	case []byte:
		return e.AppendString(b, Bytes, low.UnsafeBytesToString(v))
	default:
		r := reflect.ValueOf(v)

		return e.appendRaw(b, r, false)
	}
}

func (e *Encoder) appendRaw(b []byte, r reflect.Value, private bool) []byte {
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
		} else if private {
			return e.appendRaw(b, r.Elem(), private)
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
			if private {
				b = e.appendRaw(b, r.Index(i), private)
			} else {
				b = e.AppendValue(b, r.Index(i).Interface())
			}
		}

		return b
	case reflect.Map:
		l := r.Len()

		b = e.AppendTag(b, Map, l)

		it := r.MapRange()

		for it.Next() {
			if private {
				b = e.appendRaw(b, it.Key(), private)
				b = e.appendRaw(b, it.Value(), private)
			} else {
				b = e.AppendValue(b, it.Key().Interface())
				b = e.AppendValue(b, it.Value().Interface())
			}
		}

		return b
	case reflect.Struct:
		return e.appendStruct(b, r, private)
	case reflect.Bool:
		if r.Bool() {
			return append(b, Special|True)
		} else {
			return append(b, Special|False)
		}
	case reflect.Func:
		return append(b, Special|Undefined)
	case reflect.Uintptr:
		b = append(b, Semantic|WireHex)
		return e.AppendUint(b, Int, uint64(r.Uint()))
	case reflect.UnsafePointer:
		b = append(b, Semantic|WireHex)
		return e.AppendUint(b, Int, uint64(r.Pointer()))
	default:
		panic(r)
	}
}

func (e *Encoder) appendStruct(b []byte, r reflect.Value, private bool) []byte {
	t := r.Type()

	b = append(b, Map|LenBreak)

	b = e.appendStructFields(b, t, r, private)

	b = append(b, Special|Break)

	return b
}

func (e *Encoder) appendStructFields(b []byte, t reflect.Type, r reflect.Value, private bool) []byte {
	//	fmt.Fprintf(os.Stderr, "appendStructFields: %v\n", t)

	s := parseStruct(t)

	for _, fc := range s.fs {
		fv := r.Field(fc.I)

		if fc.OmitEmpty && fv.IsZero() {
			continue
		}

		ft := fv.Type()

		if fc.Embed && ft.Kind() == reflect.Struct {
			b = e.appendStructFields(b, ft, fv, private)

			continue
		}

		b = e.AppendString(b, String, fc.Name)

		if fc.Unexported || private {
			b = e.appendRaw(b, fv, true)
		} else {
			b = e.AppendValue(b, fv.Interface())
		}
	}

	return b
}

func (e *Encoder) AppendLoc(b []byte, pc loc.PC, cache bool) []byte {
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

func (e *Encoder) AppendFormat(b []byte, fmt string, args ...interface{}) []byte {
	b = append(b, Semantic|WireMessage)

	if len(args) == 0 {
		return e.AppendString(b, String, fmt)
	}

	b = append(b, String)

	st := len(b)

	if fmt == "" {
		b = low.AppendPrintln(b, args...)
	} else {
		b = low.AppendPrintf(b, fmt, args...)
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

func (e *Encoder) AppendFloat(b []byte, v float64) []byte {
	if q := int8(v); float64(q) == v {
		return append(b, Special|FloatInt8, byte(q))
	}

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

func (ts Timestamp) Time() (t time.Time) {
	if ts != 0 {
		t = time.Unix(0, int64(ts))
	}

	return
}
