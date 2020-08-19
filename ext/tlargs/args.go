package tlargs

import "github.com/nikandfor/tlog"

func If(c bool, a, b interface{}) interface{} {
	if c {
		return a
	} else {
		return b
	}
}

func IfV(l *tlog.Logger, tp string, a, b interface{}) interface{} {
	if l.If(tp) {
		return a
	} else {
		return b
	}
}
