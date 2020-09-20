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
func structuredFormatter(c *StructuredConfig, b []byte, sid ID, msgw int, kvs Attrs) []byte {
	const escape = `"'`

	if c == nil {
		c = &DefaultStructuredConfig
	}

	if msgw < c.MessageWidth {
		b = append(b, spaces[:c.MessageWidth-msgw]...)
	}

	pad := false
	if sid != (ID{}) {
		b = append(b, "span"...)
		b = append(b, c.KVSeparator...)

		st := len(b)
		b = append(b, "________________________________"[:c.IDWidth]...)
		sid.FormatTo(b[st:], 'f')

		pad = true
	}

	for i, kv := range kvs {
		if pad {
			b = append(b, c.PairSeparator...)
		} else {
			pad = true
		}

		b = append(b, kv.Name...)

		b = append(b, c.KVSeparator...)

		vst := len(b)

		switch v := kv.Value.(type) {
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
			b = AppendPrintf(b, "%v", kv.Value)
		}

		vend := len(b)

		vw := vend - vst
		if vw < c.ValueMaxPadWidth && i+1 < len(kvs) {
			var w int
			iw, ok := structValWidth.Load(kv.Name)
			if ok {
				w = iw.(int)
			}

			if !ok || vw > w {
				structValWidth.Store(kv.Name, vw)
			} else if vw < w {
				b = append(b, spaces[:w-vw]...)
			}
		}
	}

	return b
}
