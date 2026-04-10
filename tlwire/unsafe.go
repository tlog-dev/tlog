package tlwire

import (
	"reflect"
	"unsafe"
)

func valueInterface(r reflect.Value) any {
	return *(*any)(unsafe.Pointer(&r))
}

func unpack(x interface{}) eface {
	return *(*eface)(unsafe.Pointer(&x))
}
