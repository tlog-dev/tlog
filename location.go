package tlog

import (
	"fmt"
	"path"
	"runtime"
	"strings"
)

type Location uintptr

func Caller(s int) Location {
	var pc [1]uintptr
	runtime.Callers(2+s, pc[:])
	return Location(pc[0])
}

func Funcentry(s int) Location {
	var pc [1]uintptr
	runtime.Callers(2+s, pc[:])
	return Location(Location(pc[0]).Entry())
}

func (l Location) String() string {
	_, file, line := l.NameFileLine()
	return fmt.Sprintf("%v:%d", path.Base(file), line)
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
		return path.Base(fn)
	}
	tp = tp[p+1:]
	goto again
}
