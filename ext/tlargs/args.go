package tlargs

import "github.com/nikandfor/tlog"

//nolint:golint
func If(c bool, a, b interface{}) interface{} {
	if c {
		return a
	} else {
		return b
	}
}

//nolint:golint
func IfV(l *tlog.Logger, tp string, a, b interface{}) interface{} {
	if l.If(tp) {
		return a
	} else {
		return b
	}
}
