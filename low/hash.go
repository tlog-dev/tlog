package low

import "unsafe"

//go:noescape
//go:linkname strhash runtime.strhash
func strhash(p unsafe.Pointer, h uintptr) uintptr

//go:noescape
//go:linkname MemHash runtime.memhash

// MemHash is fast builtin hash function.
func MemHash(p unsafe.Pointer, h, s uintptr) uintptr

//go:noescape
//go:linkname MemHash64 runtime.memhash64

// MemHash64 is fast builtin hash function.
func MemHash64(p unsafe.Pointer, h uintptr) uintptr

//go:noescape
//go:linkname MemHash32 runtime.memhash32

// MemHash32 is fast builtin hash function.
func MemHash32(p unsafe.Pointer, h uintptr) uintptr

// StrHash is fast builtin hash function.
func StrHash(s string, h uintptr) uintptr {
	return strhash(unsafe.Pointer(&s), h)
}

// BytesHash is fast builtin hash function.
func BytesHash(s []byte, h uintptr) uintptr {
	return strhash(unsafe.Pointer(&s), h)
}
