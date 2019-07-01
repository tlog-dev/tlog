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

func location_test() {
	var pc [20]uintptr
	n := runtime.Callers(0, pc[:])

	fmt.Printf("-- []PC\n")
	for i, pc := range pc[:n] {
		f := runtime.FuncForPC(pc)
		fn, l := f.FileLine(pc - 1)
		fmt.Printf("%d %v %v %v\n", i, f.Name(), fn, l)
	}

	fmt.Printf("-- frames\n")
	f := runtime.CallersFrames(pc[:n])
	i := 0
	for {
		fr, more := f.Next()
		fmt.Printf("%d %v %v %v\n", i, fr.Function, fr.File, fr.Line)
		i++

		if !more {
			break
		}
	}

	fmt.Printf("-- scarry\n")
	for i, pc := range pc[:n] {
		n, f, l := Location(pc).NameFileLine()
		fmt.Printf("%d %v %v %v\n", i, n, f, l)
	}
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
