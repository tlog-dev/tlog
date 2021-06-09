package low

import "unsafe"

//go:linkname fastrandseed runtime.fastrandseed
var fastrandseed uintptr

//go:linkname Fastrand runtime.fastrand
func Fastrand() uint32

var RunID string

func init() {
	const h = "0123456789abcdef"
	var b [16]byte
	s := int(unsafe.Sizeof(fastrandseed))

	q := fastrandseed
	for i := 2*s - 1; i >= 0; i-- {
		b[i] = h[q&0xf]
		q >>= 4
	}

	RunID = string(b[:s])
}

type eface struct {
	t, p unsafe.Pointer
}

func IsNil(v interface{}) bool {
	e := *(*eface)(unsafe.Pointer(&v))

	return e.t == nil || e.p == nil
}

func InterfaceData(v interface{}) unsafe.Pointer {
	return ((*eface)(unsafe.Pointer(&v))).p
}
