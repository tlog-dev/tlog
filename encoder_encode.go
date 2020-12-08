//+build ignore

package tlog

import (
	"fmt"
	"math"
	"reflect"
	"time"

	"github.com/nikandfor/loc"
	"github.com/nikandfor/tlog/low"
)

func (e *Encoder) EncodeValue(v interface{}) {
	switch v := v.(type) {
	case nil:
		e.b = append(e.b, Special|Null)
	case string:
		e.EncodeString(String, v)
	case int:
		e.EncodeUint(Int, uint64(v))
	case float64:
		e.EncodeFloat(v)
	case Timestamp:
		e.b = append(e.b, Semantic|WireTime)
		e.EncodeUint(Int, uint64(v))
	case time.Time:
		e.b = append(e.b, Semantic|WireTime)
		e.EncodeUint(Int, uint64(v.UnixNano()))
	case time.Duration:
		e.b = append(e.b, Semantic|WireDuration)
		e.EncodeUint(Int, uint64(v.Nanoseconds()))
	case ID:
		e.EncodeID(v)
	case loc.PC:
		e.EncodePC(v, true)
	case Format:
		e.EncodeFormat(v)
	case Labels:
		e.EncodeLabels(v)
	case error:
		e.b = append(e.b, Semantic|WireError)
		e.EncodeString(String, v.Error())
	case fmt.Stringer:
		e.EncodeString(String, v.String())
	case []byte:
		e.EncodeString(Bytes, low.UnsafeBytesToString(v))
	default:
		r := reflect.ValueOf(v)
		e.encodeRaw(r)
	}
}

func (e *Encoder) encodeRaw(r reflect.Value) {
	switch r.Kind() {
	case reflect.String:
		e.EncodeString(String, r.String())
	case reflect.Int, reflect.Int64, reflect.Int32, reflect.Int16, reflect.Int8:
		e.EncodeInt(r.Int())
	case reflect.Uint, reflect.Uint64, reflect.Uint32, reflect.Uint16, reflect.Uint8:
		e.EncodeUint(Int, r.Uint())
	case reflect.Float64, reflect.Float32:
		e.EncodeFloat(r.Float())
	case reflect.Ptr, reflect.Interface:
		if r.IsNil() {
			e.b = append(e.b, Special|Null)
		} else {
			e.EncodeValue(r.Elem().Interface())
		}
	case reflect.Slice, reflect.Array:
		if r.Kind() == reflect.Slice && r.Type().Elem().Kind() == reflect.Uint8 {
			e.EncodeString(Bytes, low.UnsafeBytesToString(r.Bytes()))
			return
		}

		l := r.Len()

		e.EncodeTag(Array, l)

		for i := 0; i < l; i++ {
			e.EncodeValue(r.Index(i).Interface())
		}
	case reflect.Map:
		l := r.Len()

		e.EncodeTag(Map, l)

		it := r.MapRange()

		for it.Next() {
			e.EncodeValue(it.Key().Interface())
			e.EncodeValue(it.Value().Interface())
		}
	case reflect.Struct:
		e.b = e.appendStruct(e.b, r)
	default:
		panic(r)
	}
}

func (e *Encoder) EncodePC(pc loc.PC, cache bool) {
	e.b = append(e.b, Semantic|WireLocation)

	if e == nil || e.ls == nil || !cache {
		e.EncodeUint(Int, uint64(pc))
		return
	}

	if _, ok := e.ls[pc]; ok {
		e.EncodeUint(Int, uint64(pc))
		return
	}

	e.b = append(e.b, Map|4)

	e.EncodeString(String, "p")
	e.EncodeUint(Int, uint64(pc))

	name, file, line := pc.NameFileLine()

	e.EncodeString(String, "n")
	e.EncodeString(String, name)

	e.EncodeString(String, "f")
	e.EncodeString(String, file)

	e.EncodeString(String, "l")
	e.EncodeInt(int64(line))

	e.ls[pc] = struct{}{}
}

func (e *Encoder) EncodeLabels(ls Labels) {
	e.b = append(e.b, Semantic|WireLabels)
	e.EncodeTag(Array, len(ls))

	for _, l := range ls {
		e.EncodeString(String, l)
	}
}

func (e *Encoder) EncodeID(id ID) {
	e.b = append(e.b, Semantic|WireID)
	e.b = append(e.b, Bytes|16)
	e.b = append(e.b, id[:]...)
}

func (e *Encoder) EncodeString(tag byte, s string) {
	e.EncodeTag(tag, len(s))
	e.b = append(e.b, s...)
}

func (e *Encoder) EncodeFormat(m Format) {
	if len(m.Args) == 0 {
		e.EncodeString(String, m.Fmt)
		return
	}

	e.b = append(e.b, String)

	st := len(e.b)

	if m.Fmt == "" {
		e.b = low.AppendPrintln(e.b, m.Args...)
	} else {
		e.b = low.AppendPrintf(e.b, m.Fmt, m.Args...)
	}

	l := len(e.b) - st

	if l < Len1 {
		e.b[st-1] |= byte(l)

		return
	}

	//	fmt.Fprintf(os.Stderr, "msg before % 2x\n", b[st-1:])

	e.b = e.insertLen(e.b, st, l)

	//	fmt.Fprintf(os.Stderr, "msg after  % 2x\n", b[st-1:])
}

func (e *Encoder) EncodeFloat(v float64) {
	if q := float32(v); float64(q) == v {
		r := math.Float32bits(q)

		e.b = append(e.b, Special|Float32, byte(r>>24), byte(r>>16), byte(r>>8), byte(r))
	}

	r := math.Float64bits(v)

	e.b = append(e.b, Special|Float64, byte(r>>56), byte(r>>48), byte(r>>40), byte(r>>32), byte(r>>24), byte(r>>16), byte(r>>8), byte(r))
}

func (e *Encoder) EncodeTag(tag byte, v int) {
	switch {
	case v < Len1:
		e.b = append(e.b, tag|byte(v))
	case v <= 0xff:
		e.b = append(e.b, tag|Len1, byte(v))
	case v <= 0xffff:
		e.b = append(e.b, tag|Len2, byte(v>>8), byte(v))
	case v <= 0xffff_ffff:
		e.b = append(e.b, tag|Len4, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	default:
		e.b = append(e.b, tag|Len8, byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32), byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	}
}

func (e *Encoder) EncodeInt(v int64) {
	if v < 0 {
		e.EncodeUint(Neg, uint64(-v))
	}

	e.EncodeUint(Int, uint64(v))
}

func (e *Encoder) EncodeUint(tag byte, v uint64) {
	switch {
	case v < Len1:
		e.b = append(e.b, tag|byte(v))
	case v <= 0xff:
		e.b = append(e.b, tag|Len1, byte(v))
	case v <= 0xffff:
		e.b = append(e.b, tag|Len2, byte(v>>8), byte(v))
	case v <= 0xffff_ffff:
		e.b = append(e.b, tag|Len4, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	default:
		e.b = append(e.b, tag|Len8, byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32), byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	}
}
