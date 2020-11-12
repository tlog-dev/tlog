package loc

import (
	"sync"
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

	nfl struct {
		name string
		file string
		line int
	}
)

var (
	locmu sync.Mutex
	locc  = map[PC]nfl{}
)

//go:noescape
//go:linkname callers runtime.callers
func callers(skip int, pc []PC) int

//go:noescape
//go:linkname caller1 runtime.callers
func caller1(skip int, pc *PC, len, cap int) int

// NameFileLine returns function name, file and line number for location.
//
// This works only in the same binary where location was captured.
//
// This functions is a little bit modified version of runtime.(*Frames).Next().
func (l PC) NameFileLine() (name, file string, line int) {
	if l == 0 {
		return
	}

	locmu.Lock()
	c, ok := locc[l]
	locmu.Unlock()
	if ok {
		return c.name, c.file, c.line
	}

	name, file, line = l.nameFileLine()

	locmu.Lock()
	locc[l] = nfl{
		name: name,
		file: file,
		line: line,
	}
	locmu.Unlock()

	return
}

func (l PC) nameFileLine() (name, file string, line int) {
	if l == 0 {
		return
	}

	funcInfo := findfunc(l)
	if funcInfo.entry == nil {
		return
	}
	entry := *funcInfo.entry
	if uintptr(l) > entry {
		// We store the pc of the start of the instruction following
		// the instruction in question (the call or the inline mark).
		// This is done for historical reasons, and to make FuncForPC
		// work correctly for entries in the result of runtime.Callers.
		l--
	}
	name = funcname(funcInfo)
	file, line32 := funcline1(funcInfo, l, false)
	line = int(line32)
	if inldata := funcdata(funcInfo, _FUNCDATA_InlTree); inldata != nil {
		ix := pcdatavalue(funcInfo, _PCDATA_InlTreeIndex, l, nil)
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

// Entry is functions entry point.
func (l PC) Entry() PC {
	funcInfo := findfunc(l)
	if funcInfo.entry == nil {
		return 0
	}
	return PC(*funcInfo.entry)
}

// SetCache sets name, file and line for the PC.
// It allows to work with PC in another binary the same as in original.
func (l PC) SetCache(name, file string, line int) {
	locmu.Lock()
	if name == "" && file == "" {
		delete(locc, l)
	} else {
		locc[l] = nfl{
			name: name,
			file: file,
			line: line,
		}
	}
	locmu.Unlock()
}

//go:linkname findfunc runtime.findfunc
func findfunc(pc PC) funcInfo

//go:linkname funcline1 runtime.funcline1
func funcline1(f funcInfo, targetpc PC, strict bool) (file string, line int32)

//go:linkname funcname runtime.funcname
func funcname(f funcInfo) string

//go:linkname funcdata runtime.funcdata
func funcdata(f funcInfo, i uint8) unsafe.Pointer

//go:linkname pcdatavalue runtime.pcdatavalue
func pcdatavalue(f funcInfo, table int32, targetpc PC, cache unsafe.Pointer) int32

//go:linkname funcnameFromNameoff runtime.funcnameFromNameoff
func funcnameFromNameoff(f funcInfo, nameoff int32) string

func bytesToString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}
