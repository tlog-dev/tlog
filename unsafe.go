package tlog

import (
	"unsafe"
)

//nolint
type (
	funcID uint8

	funcInfo struct {
		entry *uintptr
		datap unsafe.Pointer
	}

	inlinedCall struct {
		parent   int16  // index of parent in the inltree, or < 0
		funcID   funcID // type of the called function
		_        byte
		file     int32 // fileno index into filetab
		line     int32 // line number of the call site
		func_    int32 // offset into pclntab for name of called function
		parentPc int32 // position of an instruction whose source position is the call site (offset from entry)
	}
)

//nolint
const (
	_PCDATA_InlTreeIndex = 1
	_FUNCDATA_InlTree    = 2
)

func (l Location) NameFileLine() (name, file string, line int) {
	pc := uintptr(l)

	funcInfo := findfunc(pc)
	if funcInfo.entry == nil {
		return
	}
	entry := *funcInfo.entry
	if pc > entry {
		// We store the pc of the start of the instruction following
		// the instruction in question (the call or the inline mark).
		// This is done for historical reasons, and to make FuncForPC
		// work correctly for entries in the result of runtime.Callers.
		pc--
	}
	name = funcname(funcInfo)
	file, line32 := funcline1(funcInfo, pc, false)
	line = int(line32)
	if inldata := funcdata(funcInfo, _FUNCDATA_InlTree); inldata != nil {
		ix := pcdatavalue(funcInfo, _PCDATA_InlTreeIndex, pc, nil)
		if ix >= 0 {
			inltree := (*[1 << 20]inlinedCall)(inldata)
			// Note: entry is not modified. It always refers to a real frame, not an inlined one.
			name = funcnameFromNameoff(funcInfo, inltree[ix].func_)
			// File/line is already correct.
			// TODO: remove file/line from InlinedCall?
		}
	}

	file = cropFilename(file, name)

	return
}

func (l Location) Entry() uintptr {
	pc := uintptr(l)

	funcInfo := findfunc(pc)
	if funcInfo.entry == nil {
		return 0
	}
	return *funcInfo.entry
}

//go:linkname findfunc runtime.findfunc
func findfunc(pc uintptr) funcInfo

//go:linkname funcline1 runtime.funcline1
func funcline1(f funcInfo, targetpc uintptr, strict bool) (file string, line int32)

//go:linkname funcname runtime.funcname
func funcname(f funcInfo) string

//go:linkname funcdata runtime.funcdata
func funcdata(f funcInfo, i uint8) unsafe.Pointer

//go:linkname pcdatavalue runtime.pcdatavalue
func pcdatavalue(f funcInfo, table int32, targetpc uintptr, cache unsafe.Pointer) int32

//go:linkname funcnameFromNameoff runtime.funcnameFromNameoff
func funcnameFromNameoff(f funcInfo, nameoff int32) string
