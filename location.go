package tlog

import (
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"
)

// Location is a program counter alias.
// Function name, file name and line can be obtained from it but only in the same binary where Caller of Funcentry was called.
type Location uintptr

// Trace is a stack trace.
// It's quiet the same as runtime.CallerFrames but more efficient.
type Trace []Location

// Caller returns information about the calling goroutine's stack. The argument s is the number of frames to ascend, with 0 identifying the caller of Caller.
//
// It's hacked version of runtime.Caller with no allocs.
func Caller(s int) Location {
	var pc [1]uintptr
	runtime.Callers(2+s, pc[:])
	return Location(pc[0])
}

// Funcentry returns information about the calling goroutine's stack. The argument s is the number of frames to ascend, with 0 identifying the caller of Caller.
//
// It's hacked version of runtime.Callers -> runtime.CallersFrames -> Frames.Next -> Frame.Entry with no allocs.
func Funcentry(s int) Location {
	var pc [1]uintptr
	runtime.Callers(2+s, pc[:])
	return Location(Location(pc[0]).Entry())
}

// StackTrace returns callers stack trace.
//
// It's hacked version of runtime.Callers -> runtime.CallersFrames -> Frames.Next -> Frame.Entry with only one alloc (resulting slice).
func StackTrace(skip, n int) Trace {
	tr := make([]Location, n)
	return StackTraceFill(1+skip, tr)
}

// StackTraceFill returns callers stack trace into provided array.
//
// It's hacked version of runtime.Callers -> runtime.CallersFrames -> Frames.Next -> Frame.Entry with no allocs.
func StackTraceFill(skip int, tr Trace) Trace {
	pc := *(*[]uintptr)(unsafe.Pointer(&tr))
	n := runtime.Callers(2+skip, pc)
	return tr[:n]
}

// String formats Location as base_name.go:line.
//
// Works only in the same binary where Caller of Funcentry was called.
func (l Location) String() string {
	_, file, line := l.NameFileLine()
	file = filepath.Base(file)

	b := []byte(file)
	i := len(b)
	b = append(b, ":        "...)

	n := 1
	for q := line; q != 0; q /= 10 {
		n++
	}

	b = b[:i+n]

	for q, j := line, n-1; j >= 1; j-- {
		b[i+j] = byte(q%10) + '0'
		q /= 10
	}

	return string(b)
}

// String formats Trace as list of type_name (file.go:line)
//
// Works only in the same binary where Caller of Funcentry was called.
func (t Trace) String() string {
	var b []byte
	for _, l := range t {
		n, f, l := l.NameFileLine()
		n = path.Base(n)
		b = AppendPrintf(b, "%-60s  at %s:%d\n", n, f, l)
	}
	return string(b)
}

func cropFilename(fn, tp string) string {
	p := strings.LastIndexByte(tp, '/')
	pp := strings.IndexByte(tp[p+1:], '.')
	tp = tp[:p+pp]

again:
	if p = strings.Index(fn, tp); p != -1 {
		return fn[p:]
	}

	p = strings.IndexByte(tp, '/')
	if p == -1 {
		return filepath.Base(fn)
	}
	tp = tp[p+1:]
	goto again
}
