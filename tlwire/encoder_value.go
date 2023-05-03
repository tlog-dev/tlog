package tlwire

import (
	"fmt"
	"net/url"
	"reflect"
	"time"
	"unsafe"

	"github.com/nikandfor/loc"

	"github.com/nikandfor/tlog/low"
)

type (
	TlogAppender interface {
		TlogAppend(b []byte) []byte
	}

	ptrSet map[unsafe.Pointer]struct{}

	ValueEncoder func(e *Encoder, b []byte, val interface{}) []byte

	//nolint:structcheck
	eface struct {
		typ unsafe.Pointer
		ptr unsafe.Pointer
	}

	//nolint:structcheck
	reflectValue struct {
		typ  unsafe.Pointer
		ptr  unsafe.Pointer
		flag uintptr
	}

	encoders map[unsafe.Pointer]ValueEncoder
)

var defaultEncoders = encoders{}

func SetEncoder(tp interface{}, encoder ValueEncoder) {
	defaultEncoders.Set(tp, encoder)
}

func (e *Encoder) SetEncoder(tp interface{}, encoder ValueEncoder) {
	if e.custom == nil {
		e.custom = encoders{}
	}

	e.custom.Set(tp, encoder)
}

func (e encoders) Set(tp interface{}, encoder ValueEncoder) {
	if tp == nil {
		panic("nil type")
	}

	ef := *(*eface)(unsafe.Pointer(&tp))

	e[ef.typ] = encoder

	if encoder == nil {
		delete(e, ef.typ)
	}
}

func init() {
	SetEncoder(loc.PC(0), func(e *Encoder, b []byte, x interface{}) []byte {
		return Encoder{}.AppendCaller(b, x.(loc.PC))
	})
	SetEncoder(loc.PCs(nil), func(e *Encoder, b []byte, x interface{}) []byte {
		return Encoder{}.AppendCallers(b, x.(loc.PCs))
	})

	SetEncoder(time.Time{}, func(e *Encoder, b []byte, x interface{}) []byte {
		return Encoder{}.AppendTimeTZ(b, x.(time.Time))
	})
	SetEncoder((*time.Time)(nil), func(e *Encoder, b []byte, x interface{}) []byte {
		return Encoder{}.AppendTimeTZ(b, *x.(*time.Time))
	})

	SetEncoder(time.Duration(0), func(e *Encoder, b []byte, x interface{}) []byte {
		return Encoder{}.AppendDuration(b, x.(time.Duration))
	})
	SetEncoder((*time.Duration)(nil), func(e *Encoder, b []byte, x interface{}) []byte {
		return Encoder{}.AppendDuration(b, *x.(*time.Duration))
	})

	SetEncoder((*url.URL)(nil), func(e *Encoder, b []byte, x interface{}) []byte {
		u := x.(*url.URL)
		if u == nil {
			return Encoder{}.AppendNil(b)
		}

		return Encoder{}.AppendString(b, u.String())
	})
}

func (e *Encoder) AppendKeyValue(b []byte, key string, v interface{}) []byte {
	b = e.AppendKey(b, key)
	b = e.AppendValue(b, v)
	return b
}

//go:linkname appendValue github.com/nikandfor/tlog/tlwire.(*Encoder).appendValue
//go:noescape
func appendValue(e *Encoder, b []byte, v interface{}) []byte

func (e *Encoder) AppendValue(b []byte, v interface{}) []byte {
	return appendValue(e, b, v)
}

func (e *Encoder) AppendValueSafe(b []byte, v interface{}) []byte {
	return e.appendValue(b, v)
}

// Called through linkname hack as appendValue from (Encoder).AppendValue.
func (e *Encoder) appendValue(b []byte, v interface{}) []byte {
	if v == nil {
		return append(b, Special|Nil)
	}

	r := reflect.ValueOf(v)

	return e.appendRaw(b, r, ptrSet{})
}

func (e *Encoder) appendRaw(b []byte, r reflect.Value, visited ptrSet) []byte { //nolint:gocognit,cyclop
	if r.CanInterface() {
		//	v := r.Interface()
		v := valueInterface(r)

		//	if r.Type().Comparable() && v != r.Interface() {
		//		panic(fmt.Sprintf("not equal interface %v: %x %v %v", r, value(r), raweface(v), raweface(r.Interface())))
		//	}

		ef := raweface(v)

		if e != nil {
			if enc, ok := e.custom[ef.typ]; ok {
				return enc(e, b, v)
			}
		}

		if enc, ok := defaultEncoders[ef.typ]; ok {
			return enc(e, b, v)
		}

		switch v := v.(type) {
		case TlogAppender:
			return v.TlogAppend(b)
		case interface {
			ProtoMessage()
		}:
		case error:
			return e.AppendError(b, v)
		case fmt.Stringer:
			return e.AppendString(b, v.String())
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
		} else { //nolint:golint
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

func value(v reflect.Value) reflectValue {
	return *(*reflectValue)(unsafe.Pointer(&v))
}

func valueInterface(r reflect.Value) interface{} {
	v := value(r)

	if r.Kind() == reflect.Interface {
		// Special case: return the element inside the interface.
		// Empty interface has one layout, all interfaces with
		// methods have a second layout.
		if r.NumMethod() == 0 {
			return *(*interface{})(v.ptr)
		}
		return *(*interface {
			M()
		})(v.ptr)
	}

	const flagAddr = 1 << 8

	v.flag &^= flagAddr

	return reflect_packEface(v)
}

//go:linkname reflect_packEface reflect.packEface
func reflect_packEface(reflectValue) interface{}

func raweface(x interface{}) eface {
	return *(*eface)(unsafe.Pointer(&x))
}
