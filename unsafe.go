package tlog

import (
	_ "unsafe"

	"github.com/nikandfor/loc"
)

//go:noescape
//go:linkname caller1 runtime.callers
func caller1(skip int, pc *loc.PC, len, cap int) int
