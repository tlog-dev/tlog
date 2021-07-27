package wire

import (
	"fmt"
	"reflect"
	"time"
	"unsafe"

	"github.com/nikandfor/loc"

	"github.com/nikandfor/tlog/low"
)

type (
	TlogAppender interface {
		TlogAppend(e *Encoder, b []byte) []byte
	}

	TlogParser interface {
		TlogParse(d *Decoder, b []byte, st int) int
	}

	ptrSet map[unsafe.Pointer]struct{}
)

//go:linkname appendValue github.com/nikandfor/tlog/wire.(*Encoder).appendValue
//go:noescape
func appendValue(e *Encoder, b []byte, v interface{}, visited ptrSet) []byte

func (e *Encoder) AppendValue(b []byte, v interface{}) []byte {
	return appendValue(e, b, v, nil)
}

func init() {
	_ = (&Encoder{}).appendValue(nil, nil, ptrSet{})
}

// called through linkname hack.
func (e *Encoder) appendValue(b []byte, v interface{}, visited ptrSet) []byte {
	if low.IsNil(v) {
		return append(b, Special|Null)
	}

	switch v := v.(type) {
	case string:
		b = e.AppendString(b, String, v)
	case int:
		b = e.AppendSigned(b, int64(v))
	case float64:
		b = e.AppendFloat(b, v)
	case time.Time:
		b = e.AppendTime(b, v)
	case time.Duration:
		b = e.AppendDuration(b, v)
	case TlogAppender:
		b = v.TlogAppend(e, b)
	case loc.PC:
		b = e.AppendPC(b, v, true)
	case loc.PCs:
		b = e.AppendPCs(b, v, true)
	case error:
		b = append(b, Semantic|Error)
		b = e.AppendString(b, String, v.Error())
	case fmt.Stringer:
		b = e.AppendString(b, String, v.String())
	default:
		r := reflect.ValueOf(v)

		b = e.appendRaw(b, r, false, visited)
	}

	return b
}

func (e *Encoder) appendRaw(b []byte, r reflect.Value, private bool, visited ptrSet) []byte { //nolint:gocognit
	switch r.Kind() {
	case reflect.String:
		return e.AppendString(b, String, r.String())
	case reflect.Int, reflect.Int64, reflect.Int32, reflect.Int16, reflect.Int8:
		return e.AppendSigned(b, r.Int())
	case reflect.Uint, reflect.Uint64, reflect.Uint32, reflect.Uint16, reflect.Uint8:
		return e.AppendInt(b, Int, r.Uint())
	case reflect.Float64, reflect.Float32:
		return e.AppendFloat(b, r.Float())
	case reflect.Ptr, reflect.Interface:
		if r.IsNil() {
			return append(b, Special|Null)
		}

		if r.Kind() == reflect.Ptr {
			ptr := unsafe.Pointer(r.Pointer())

			if visited == nil {
				visited = make(map[unsafe.Pointer]struct{})
			}

			if _, ok := visited[ptr]; ok {
				return append(b, Special|Undefined)
			}

			visited[ptr] = struct{}{}
		}

		if private {
			return e.appendRaw(b, r.Elem(), private, visited)
		}

		return e.appendValue(b, r.Elem().Interface(), visited)
	case reflect.Slice, reflect.Array:
		if r.Type().Elem().Kind() == reflect.Uint8 {
			if r.Kind() == reflect.Array {
				if r.CanAddr() {
					r = r.Slice(0, r.Len())
				} else {
					return e.AppendString(b, Bytes, low.UnsafeString(low.InterfaceData(r.Interface()), r.Len()))
				}
			}

			return e.AppendString(b, Bytes, low.UnsafeBytesToString(r.Bytes()))
		}

		l := r.Len()

		b = e.AppendTag(b, Array, int64(l))

		for i := 0; i < l; i++ {
			if private {
				b = e.appendRaw(b, r.Index(i), private, visited)
			} else {
				b = e.appendValue(b, r.Index(i).Interface(), visited)
			}
		}

		return b
	case reflect.Map:
		l := r.Len()

		b = e.AppendTag(b, Map, int64(l))

		it := r.MapRange()

		for it.Next() {
			if private {
				b = e.appendRaw(b, it.Key(), private, visited)
				b = e.appendRaw(b, it.Value(), private, visited)
			} else {
				b = e.appendValue(b, it.Key().Interface(), visited)
				b = e.appendValue(b, it.Value().Interface(), visited)
			}
		}

		return b
	case reflect.Struct:
		return e.appendStruct(b, r, private, visited)
	case reflect.Bool:
		if r.Bool() {
			return append(b, Special|True)
		} else { //nolint:golint,revive
			return append(b, Special|False)
		}
	case reflect.Func:
		return append(b, Special|Undefined)
	case reflect.Uintptr:
		return e.AppendInt(b, Int, r.Uint())
	case reflect.UnsafePointer:
		return e.AppendInt(b, Int, uint64(r.Pointer()))
	default:
		panic(r)
	}
}

func (e *Encoder) appendStruct(b []byte, r reflect.Value, private bool, visited ptrSet) []byte {
	t := r.Type()

	b = append(b, Map|LenBreak)

	b = e.appendStructFields(b, t, r, private, visited)

	b = append(b, Special|Break)

	return b
}

func (e *Encoder) appendStructFields(b []byte, t reflect.Type, r reflect.Value, private bool, visited ptrSet) []byte {
	//	fmt.Fprintf(os.Stderr, "appendStructFields: %v  ctx %p %d\n", t, visited, len(visited))

	s := parseStruct(t)

	for _, fc := range s.fs {
		fv := r.Field(fc.I)

		if fc.OmitEmpty && fv.IsZero() {
			continue
		}

		ft := fv.Type()

		if fc.Embed && ft.Kind() == reflect.Struct {
			b = e.appendStructFields(b, ft, fv, private, visited)

			continue
		}

		b = e.AppendString(b, String, fc.Name)

		if k := fv.Kind(); (k == reflect.Ptr || k == reflect.Interface) && fv.IsNil() {
			b = append(b, Special|Null)

			continue
		}

		if fc.Hex {
			b = append(b, Semantic|Hex)
		}

		if fc.Unexported || private {
			b = e.appendRaw(b, fv, true, visited)
		} else {
			b = e.appendValue(b, fv.Interface(), visited)
		}
	}

	return b
}
