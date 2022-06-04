package low

import "unsafe"

type (
	eface struct {
		t unsafe.Pointer
		p unsafe.Pointer
	}

	sh struct {
		p unsafe.Pointer
		l int
	}
)

func UnpackInterface(v interface{}) (t, p unsafe.Pointer) {
	e := ((*eface)(unsafe.Pointer(&v)))
	return e.t, e.p
}

func InterfaceType(v interface{}) unsafe.Pointer {
	return ((*eface)(unsafe.Pointer(&v))).t
}

func InterfaceData(v interface{}) unsafe.Pointer {
	return ((*eface)(unsafe.Pointer(&v))).p
}

func UnsafeString(ptr unsafe.Pointer, l int) string {
	return *(*string)(unsafe.Pointer(&sh{p: ptr, l: l}))
}
