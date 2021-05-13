package tlog

import (
	"unsafe"

	"github.com/nikandfor/loc"
	"github.com/nikandfor/tlog/wire"
)

//go:linkname appendKVs0 github.com/nikandfor/tlog.appendKVs
//go:noescape
func appendKVs0(e *wire.Encoder, b []byte, kvs []interface{}) []byte

//go:noescape
//go:linkname caller1 runtime.callers
func caller1(skip int, pc *loc.PC, len, cap int) int

//go:linkname UnixNano github.com/nikandfor/tlog/low.UnixNano
func UnixNano() Timestamp

func stringToBytes(s string) []byte {
	return *(*[]byte)(unsafe.Pointer(&s))
}
