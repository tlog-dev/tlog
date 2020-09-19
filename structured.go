package tlog

import (
	"bytes"
	"strconv"
	"strings"
	"sync"
)

type (
	StructuredConfig struct {
		// Minimal message width
		MessageWidth     int
		IDWidth          int
		ValueMaxPadWidth int

		PairSeparator string
		KVSeparator   string

		QuoteAnyValue   bool
		QuoteEmptyValue bool
	}
)

var DefaultStructuredConfig = StructuredConfig{
	MessageWidth:     40,
	IDWidth:          8,
	ValueMaxPadWidth: 20,
	PairSeparator:    "  ",
	KVSeparator:      "=",
}

var structValWidth sync.Map // string -> int

//nolint:gocognit
func structuredFormatter(l *Logger, b []byte, sid ID, msg string, kv []interface{}) []byte {
	const escape = `"'`

	c := l.StructuredConfig
	if c == nil {
		c = &DefaultStructuredConfig
	}

	if len(kv)&1 == 1 {
		panic("bad kv: pairs expected")
	}

	b = AppendPrintf(b, "%-*s", c.MessageWidth, msg)

	pad := false
	if sid != (ID{}) {
		b = append(b, "span"...)
		b = append(b, c.KVSeparator...)

		st := len(b)
		b = append(b, "________________________________"[:c.IDWidth]...)
		sid.FormatTo(b[st:], 'f')

		pad = true
	}

	for i := 0; i < len(kv); i += 2 {
		if pad {
			b = append(b, c.PairSeparator...)
		} else {
			pad = true
		}

		kst := len(b)

		switch k := kv[i].(type) {
		case string:
			b = append(b, k...)
		case []byte:
			b = append(b, k...)
		default:
			panic("bad kv: expected key")
		}

		kend := len(b)

		b = append(b, c.KVSeparator...)

		vst := len(b)

		switch v := kv[i+1].(type) {
		case string:
			if c.QuoteAnyValue || c.QuoteEmptyValue && v == "" || strings.Contains(v, c.KVSeparator) || strings.ContainsAny(v, escape) {
				b = strconv.AppendQuote(b, v)
			} else {
				b = append(b, v...)
			}
		case []byte:
			if c.QuoteAnyValue || c.QuoteEmptyValue && len(v) == 0 || bytes.Contains(v, []byte(c.KVSeparator)) || bytes.ContainsAny(v, escape) {
				b = strconv.AppendQuote(b, string(v))
			} else {
				b = append(b, v...)
			}
		default:
			b = AppendPrintf(b, "%v", kv[i+1])
		}

		vend := len(b)

		var kh uintptr

		vw := vend - vst
		if vw < c.ValueMaxPadWidth && i+2 < len(kv) {
			k := b[kst:kend]
			kh = byteshash(&k, 0)

			var w int
			iw, ok := structValWidth.Load(kh)
			if ok {
				w = iw.(int)
			}

			if !ok || vw > w {
				structValWidth.Store(kh, vw)
			} else if vw < w {
				b = append(b, spaces[:w-vw]...)
			}
		}
	}

	return b
}
