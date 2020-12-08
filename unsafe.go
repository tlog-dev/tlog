package tlog

import (
	_ "unsafe"

	"github.com/nikandfor/loc"
)

//go:linkname append0 github.com/nikandfor/tlog.append1
//go:noescape
func append0(b []interface{}, v ...interface{}) []interface{}

//go:linkname encodeKVs0 github.com/nikandfor/tlog.(*Encoder).encodeKVs
//go:noescape
func encodeKVs0(e *Encoder, kvs ...interface{})

//go:noescape
//go:linkname caller1 runtime.callers
func caller1(skip int, pc *loc.PC, len, cap int) int
