package tlog

import (
	"fmt"
	"path"
	"runtime"
	"strings"
)

type Location uintptr

func location(s int) Location {
	var pc [1]uintptr
	runtime.Callers(2+s, pc[:])
	return Location(pc[0])
}

func funcentry(s int) Location {
	var pc [1]uintptr
	runtime.Callers(2+s, pc[:])
	return Location(Location(pc[0]).Entry())
}

func (l Location) Short() string {
	_, file, line := l.NameFileLine()
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
