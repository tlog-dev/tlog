package tlwire

import (
	"reflect"
	"strings"
	"sync"
)

type (
	rawStruct struct {
		fs []rawField
	}

	rawField struct {
		Name       string
		TagName    string
		Idx        int
		OmitEmpty  bool
		Unexported bool
		Embed      bool
		Hex        bool
	}
)

var (
	structsMu    sync.Mutex
	structsCache = map[reflect.Type]*rawStruct{}
)

func parseStruct(tp reflect.Type) (s *rawStruct) { //nolint:gocognit
	defer structsMu.Unlock()
	structsMu.Lock()

	s = structsCache[tp]
	if s != nil {
		return s
	}

	s = &rawStruct{}
	structsCache[tp] = s

	ff := tp.NumField()

	for i := range ff {
		f := tp.Field(i)

		sf := rawField{
			Idx:        i,
			Unexported: f.PkgPath != "",
		}

		tag, ok := f.Tag.Lookup("tlog")

		if !ok {
			switch f.Type.Kind() {
			case reflect.Chan, reflect.Func, reflect.UnsafePointer:
				continue
			}

			if f.PkgPath != "" {
				continue
			}
		}

		if tag == "" {
			tag = f.Tag.Get("yaml")
		}
		if tag == "" {
			tag = f.Tag.Get("json")
		}

		ss := strings.Split(tag, ",")

		if len(ss) != 0 {
			if ss[0] == "-" {
				continue
			}

			if ss[0] != "" {
				sf.Name = ss[0]
				sf.TagName = ss[0]
			}
		}

		for _, s := range ss[1:] {
			switch s {
			case "omitempty":
				sf.OmitEmpty = true
			case "embed":
				sf.Embed = true
			case "hex":
				sf.Hex = true
			}
		}

		if sf.Name == "" {
			sf.Name = f.Name
		}

		s.fs = append(s.fs, sf)
	}

	return s
}
