package low

import "unsafe"

//go:noescape
//go:linkname strhash runtime.strhash
func strhash(p *string, h uintptr) uintptr

//go:noescape
//go:linkname byteshash runtime.strhash
func byteshash(p *[]byte, h uintptr) uintptr

//go:noescape
//go:linkname memhash runtime.memhash
func memhash(p *byte, h uintptr, s int) uintptr

//go:noescape
//go:linkname MemHash runtime.memhash
func MemHash(p unsafe.Pointer, h, s uintptr) uintptr

//go:noescape
//go:linkname MemHash64 runtime.memhash64
func MemHash64(p unsafe.Pointer, h uintptr) uintptr

//go:noescape
//go:linkname MemHash32 runtime.memhash32
func MemHash32(p unsafe.Pointer, h uintptr) uintptr

func StrHash(s string, h uintptr) uintptr {
	return strhash(&s, h)
}

func BytesHash(s []byte, h uintptr) uintptr {
	return byteshash(&s, h)
}
