package tlog

import (
	"fmt"
	"path"
	"runtime"
	"strings"

	"github.com/nikandfor/json"
)

type Location uintptr

func location(s int) Location {
	pc, _, _, _ := runtime.Caller(s + 1)
	return Location(pc)
}

func funcentry(s int) Location {
	pc, _, _, _ := runtime.Caller(s + 1)
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

func (l Location) FileBase() string {
	f := runtime.FuncForPC(uintptr(l))
	file, _ := f.FileLine(uintptr(l))
	return path.Base(file)
}

func (l Location) MarshalJSON(w *json.Writer) error {
	f := runtime.FuncForPC(uintptr(l))
	n := f.Name()
	e := f.Entry()
	file, entry := f.FileLine(e)
	_, line := f.FileLine(uintptr(l))
	file = cropFilename(file, n)

	w.ObjStart()

	w.ObjKey([]byte("pc"))
	fmt.Fprintf(w, "%d", l)

	w.ObjKey([]byte("f"))
	w.StringString(file)

	w.ObjKey([]byte("n"))
	w.StringString(n)

	w.ObjKey([]byte("e"))
	fmt.Fprintf(w, "%d", entry)

	w.ObjKey([]byte("l"))
	fmt.Fprintf(w, "%d", line)

	w.ObjEnd()

	return w.Err()
}

func cropFilename(fn, tp string) string {
	p := strings.LastIndexByte(tp, '/')
	tp = tp[:p]

	p = strings.Index(fn, tp)

	return fn[p:]
}
