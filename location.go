package tlog

import (
	"fmt"
	"path"
	"runtime"
	"strings"
)

type Location uintptr

func location(s int) Location {
	pc, _, _, _ := runtime.Caller(1 + s)
	return Location(pc)
}

func funcentry(s int) Location {
	pc, _, _, _ := runtime.Caller(1 + s)
	f := runtime.FuncForPC(pc)
	return Location(f.Entry())
}

func (l Location) Line() int {
	f := runtime.FuncForPC(uintptr(l))
	_, line := f.FileLine(uintptr(l))
	return line
}

func (l Location) File() string {
	f := runtime.FuncForPC(uintptr(l))
	file, _ := f.FileLine(f.Entry())
	return cropFilename(file, f.Name())
}

func (l Location) FileLine() (string, int) {
	f := runtime.FuncForPC(uintptr(l))
	file, line := f.FileLine(uintptr(l))
	file = cropFilename(file, f.Name())
	return file, line
}

func (l Location) FileBase() string {
	f := runtime.FuncForPC(uintptr(l))
	file, _ := f.FileLine(uintptr(l))
	return path.Base(file)
}

func (l Location) Func() string {
	f := runtime.FuncForPC(uintptr(l))
	return path.Base(f.Name())
}

func (l Location) FuncFull() string {
	f := runtime.FuncForPC(uintptr(l))
	return f.Name()
}

func (l Location) Short() string {
	f := runtime.FuncForPC(uintptr(l))
	file, line := f.FileLine(uintptr(l))
	return fmt.Sprintf("%v:%d", path.Base(file), line)
}

func (l Location) String() string {
	return fmt.Sprintf("% 8x", uintptr(l))
}

func cropFilename(fn, tp string) string {
	p := strings.LastIndexByte(tp, '/')
	if p == -1 {
		return path.Base(fn)
	}
	tp = tp[:p]

	p = strings.Index(fn, tp)

	return fn[p:]
}
