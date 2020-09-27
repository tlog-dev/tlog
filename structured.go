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
		ValueMaxPadWidth int

		PairSeparator string
		KVSeparator   string

		QuoteAnyValue   bool
		QuoteEmptyValue bool

		structValWidth sync.Map // string -> int
	}
)

// Copy makes config copy.
// Use it instead of assignment since structure contains fields that should not be copied.
func (c *StructuredConfig) Copy() StructuredConfig {
	return StructuredConfig{
		MessageWidth:     c.MessageWidth,
		ValueMaxPadWidth: c.ValueMaxPadWidth,

		PairSeparator: c.PairSeparator,
		KVSeparator:   c.KVSeparator,

		QuoteAnyValue:   c.QuoteAnyValue,
		QuoteEmptyValue: c.QuoteEmptyValue,
	}
}

// DefaultStructuredConfig is default config to format structured logs by ConsoleWriter.
var DefaultStructuredConfig = StructuredConfig{
	MessageWidth:     40,
	ValueMaxPadWidth: 20,
	PairSeparator:    "  ",
	KVSeparator:      "=",
}

//nolint:gocognit
func structuredFormatter(c *StructuredConfig, b []byte, sid ID, msgw int, kvs Attrs) []byte {
	const escape = `"'`

	if c == nil {
		c = &DefaultStructuredConfig
	}

	if msgw < c.MessageWidth {
		b = append(b, spaces[:c.MessageWidth-msgw]...)
	}

	for i, kv := range kvs {
		if i != 0 {
			b = append(b, c.PairSeparator...)
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
			iw, ok := c.structValWidth.Load(kv.Name)
			if ok {
				w = iw.(int)
			}

			if !ok || vw > w {
				c.structValWidth.Store(kv.Name, vw)
			} else if vw < w {
				b = append(b, spaces[:w-vw]...)
			}
		}
	}

	return b
}
