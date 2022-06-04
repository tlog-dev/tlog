package main

import (
	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/tlwire"
)

type (
	ValueEncoder struct {
		b []byte
	}
)

func main() {
	var e tlwire.Encoder

	// pre encode key-value pair
	kvs := e.AppendKeyInt(nil, "raw_key_val", 4)

	tlog.Printw("raw kv pair", tlog.RawMessage(kvs))

	// pre encode value
	val1 := e.AppendString(nil, "_value_")

	tlog.Printw("raw value", "raw_value", tlog.RawMessage(val1))

	// custom value encoding
	val2 := ValueEncoder{
		b: []byte{0x00, 0x11, 0x22},
	}

	tlog.Printw("custom value encoder", "custom_formatted", val2)
}

func (x ValueEncoder) TlogAppend(b []byte) []byte {
	var e tlwire.Encoder
	b = e.AppendTag(b, tlwire.Semantic, tlwire.Hex)
	b = e.AppendBytes(b, x.b)
	return b
}
