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
	return unsafe.String((*byte)(ptr), l)
}

func UnsafeBytesToString(b []byte) string {
	return unsafe.String(unsafe.SliceData(b), len(b))
}

func NoEscapeBuffer(b []byte) []byte {
	return *(*[]byte)(noescape(unsafe.Pointer(&b)))
}

// noescape hides a pointer from escape analysis.  noescape is
// the identity function but escape analysis doesn't think the
// output depends on the input.  noescape is inlined and currently
// compiles down to zero instructions.
// USE CAREFULLY!
//
//go:nosplit
func noescape(p unsafe.Pointer) unsafe.Pointer {
	x := uintptr(p)
	return unsafe.Pointer(x ^ 0) //nolint:staticcheck
}
