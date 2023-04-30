package tlog

import (
	"unsafe"
	_ "unsafe"

	"github.com/nikandfor/loc"
)

//go:noescape
//go:linkname caller1 runtime.callers
func caller1(skip int, pc *loc.PC, len, cap int) int

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
