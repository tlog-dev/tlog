package tlwire

import (
	"reflect"
	"unsafe"

	"github.com/nikandfor/loc"
	"github.com/nikandfor/tlog/low"
)

type (
	TlogAppender interface {
		TlogAppend(b []byte) []byte
	}

	ptrSet map[unsafe.Pointer]struct{}

	ValueEncoder func(b []byte, val interface{}) []byte

	eface struct {
		typ unsafe.Pointer
		ptr unsafe.Pointer
	}
)

var encoders = map[unsafe.Pointer]ValueEncoder{}

func SetEncoder(tp interface{}, encoder ValueEncoder) {
	ef := *(*eface)(unsafe.Pointer(&tp))

	encoders[ef.typ] = encoder
}

func init() {
	SetEncoder(loc.PC(0), func(b []byte, x interface{}) []byte {
		return Encoder{}.AppendCaller(b, x.(loc.PC))
	})
}

//go:linkname appendValue github.com/nikandfor/tlog/tlwire.(*Encoder).appendValue
//go:noescape
func appendValue(e *Encoder, b []byte, v interface{}) []byte

func (e *Encoder) AppendValue(b []byte, v interface{}) []byte {
	return appendValue(e, b, v)
}

func init() {
	_ = (&Encoder{}).appendValue(nil, nil) // for linter to know it is used
}

// called through linkname hack as appendValue from (Encoder).AppendValue
func (e *Encoder) appendValue(b []byte, v interface{}) []byte {
	if v == nil {
		return append(b, Special|Nil)
	}

	r := reflect.ValueOf(v)

	return e.appendRaw(b, r, ptrSet{})
}

func (e *Encoder) appendRaw(b []byte, r reflect.Value, visited ptrSet) []byte { //nolint:gocognit
	if r.CanInterface() {
		v := r.Interface()

		if a, ok := v.(TlogAppender); ok {
			return a.TlogAppend(b)
		}

		ef := *(*eface)(unsafe.Pointer(&v))

		if enc, ok := encoders[ef.typ]; ok {
			return enc(b, v)
		}
	}

	switch r.Kind() {
	case reflect.String:
		return e.AppendString(b, r.String())
	case reflect.Int, reflect.Int64, reflect.Int32, reflect.Int16, reflect.Int8:
		return e.AppendInt64(b, r.Int())
	case reflect.Uint, reflect.Uint64, reflect.Uint32, reflect.Uint16, reflect.Uint8:
		return e.AppendUint64(b, r.Uint())
	case reflect.Float64, reflect.Float32:
		return e.AppendFloat(b, r.Float())
	case reflect.Ptr, reflect.Interface:
		if r.IsNil() {
			return append(b, Special|Nil)
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

			defer delete(visited, ptr)
		}

		r = r.Elem()

		return e.appendRaw(b, r, visited)
	case reflect.Slice, reflect.Array:
		if r.Type().Elem().Kind() == reflect.Uint8 {
			if r.Kind() == reflect.Array {
				if r.CanAddr() {
					r = r.Slice(0, r.Len())
				} else {
					return e.AppendTagString(b, Bytes, low.UnsafeString(low.InterfaceData(r.Interface()), r.Len()))
				}
			}

			return e.AppendBytes(b, r.Bytes())
		}

		l := r.Len()

		b = e.AppendTag(b, Array, l)

		for i := 0; i < l; i++ {
			b = e.appendRaw(b, r.Index(i), visited)
		}

		return b
	case reflect.Map:
		l := r.Len()

		b = e.AppendTag(b, Map, l)

		it := r.MapRange()

		for it.Next() {
			b = e.appendRaw(b, it.Key(), visited)
			b = e.appendRaw(b, it.Value(), visited)
		}

		return b
	case reflect.Struct:
		return e.appendStruct(b, r, visited)
	case reflect.Bool:
		if r.Bool() {
			return append(b, Special|True)
		} else { //nolint:golint,revive
			return append(b, Special|False)
		}
	case reflect.Func:
		return append(b, Special|Undefined)
	case reflect.Uintptr:
		b = append(b, Semantic|Hex)
		return e.AppendTag64(b, Int, r.Uint())
	case reflect.UnsafePointer:
		b = append(b, Semantic|Hex)
		return e.AppendTag64(b, Int, uint64(r.Pointer()))
	default:
		panic(r)
	}

	return nil
}

func (e *Encoder) appendStruct(b []byte, r reflect.Value, visited ptrSet) []byte {
	t := r.Type()

	b = append(b, Map|LenBreak)

	b = e.appendStructFields(b, t, r, visited)

	b = append(b, Special|Break)

	return b
}

func (e *Encoder) appendStructFields(b []byte, t reflect.Type, r reflect.Value, visited ptrSet) []byte {
	//	fmt.Fprintf(os.Stderr, "appendStructFields: %v  ctx %p %d\n", t, visited, len(visited))

	s := parseStruct(t)

	for _, fc := range s.fs {
		fv := r.Field(fc.Idx)

		if fc.OmitEmpty && fv.IsZero() {
			continue
		}

		ft := fv.Type()

		if fc.Embed && ft.Kind() == reflect.Struct {
			b = e.appendStructFields(b, ft, fv, visited)

			continue
		}

		b = e.AppendString(b, fc.Name)

		if fc.Hex {
			b = append(b, Semantic|Hex)
		}

		b = e.appendRaw(b, fv, visited)
	}

	return b
}
