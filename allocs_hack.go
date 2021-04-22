// +build !tlogsafe

package tlog

import _ "unsafe"

//go:linkname encodeKVs0 github.com/nikandfor/tlog.(*Encoder).encodeKVs
//go:noescape
func encodeKVs0(e *Encoder, kvs []interface{})
