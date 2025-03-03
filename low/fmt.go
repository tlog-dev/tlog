package low

import (
	"fmt"
	"unsafe"
)

func Append(b []byte, args ...any) []byte {
	d := unsafe.SliceData(args)
	h := (*any)(noescape(unsafe.Pointer(d)))
	r := unsafe.Slice(h, len(args))

	return fmt.Append(b, r...)
}

func Appendf(b []byte, format string, args ...any) []byte {
	d := unsafe.SliceData(args)
	h := (*any)(noescape(unsafe.Pointer(d)))
	r := unsafe.Slice(h, len(args))

	return fmt.Appendf(b, format, r...)
}

func Appendln(b []byte, args ...any) []byte {
	d := unsafe.SliceData(args)
	h := (*any)(noescape(unsafe.Pointer(d)))
	r := unsafe.Slice(h, len(args))

	return fmt.Appendln(b, r...)
}
