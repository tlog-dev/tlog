package low

import (
	"reflect"
	"strings"
	"unsafe"
)

var Printw func(args string, kvs ...interface{})

type emptyInterface struct {
	typ  unsafe.Pointer
	word unsafe.Pointer
}

//go:linkname typelinks2 reflect.typelinks
func typelinks2() (sections []unsafe.Pointer, offset [][]int32)

//go:linkname resolveTypeOff reflect.resolveTypeOff
func resolveTypeOff(rtype unsafe.Pointer, off int32) unsafe.Pointer

func LoadGoTypes(pkg string) {
	var obj interface{} = reflect.TypeOf(0)

	sections, offset := typelinks2()

	for i, offs := range offset {
		rodata := sections[i]

		for _, off := range offs {
			(*emptyInterface)(unsafe.Pointer(&obj)).word = resolveTypeOff(unsafe.Pointer(rodata), off)
			typ := obj.(reflect.Type)

			oldKind := typ.Kind()

			if typ.Kind() == reflect.Ptr {
				typ = typ.Elem()
			}

			if strings.HasPrefix(typ.PkgPath(), pkg) {
				Printw("type", "type", typ.Name(), "pkg", typ.PkgPath(), "str", typ.String(), "kind", typ.Kind(), "old_kind", oldKind)
			}
		}
	}

	//	Printw("len", "sections", len(sections), "offsets", len(offset))

	//	for i, offs := range offset {
	//		Printw("len", "off", i, "offs", len(offs))
	//	}
}
