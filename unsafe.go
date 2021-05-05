package tlog

import (
	"unsafe"

	"github.com/nikandfor/loc"
)

//go:linkname appendKVs github.com/nikandfor/tlog.(*Encoder).appendKVs
//go:noescape
func appendKVs(e *Encoder, b []byte, kvs []interface{}) []byte

//go:noescape
//go:linkname caller1 runtime.callers
func caller1(skip int, pc *loc.PC, len, cap int) int

//go:linkname UnixNano github.com/nikandfor/tlog/low.UnixNano
func UnixNano() Timestamp

func stringToBytes(s string) []byte {
	return *(*[]byte)(unsafe.Pointer(&s))
}

type eface struct {
	typ  uintptr
	data uintptr
}
