package main

import (
	"flag"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/ext/tlflag"
	"github.com/nikandfor/tlog/wire"
)

type (
	ValueEncoder struct {
		b []byte
	}
)

var (
	out = flag.String("o", "stderr+dm", "log output")
)

func main() {
	flag.Parse()

	w, err := tlflag.OpenWriter(*out)
	if err != nil {
		panic(err)
	}

	tlog.DefaultLogger = tlog.New(w)

	var e wire.Encoder

	// pre encode key-value pair
	kvs := e.AppendKeyInt(nil, "raw_key_val", 4)

	tlog.Printw("raw kv pair", tlog.RawMessage(kvs))

	// pre encode value
	val1 := e.AppendString(nil, wire.String, "_value_")

	tlog.Printw("raw value", "raw_value", tlog.RawMessage(val1))

	// custom value encoding
	val2 := ValueEncoder{
		b: []byte{0x00, 0x11, 0x22},
	}

	tlog.Printw("custom value encoder", "custom_formatted", val2)
}

func (x ValueEncoder) TlogAppend(e *wire.Encoder, b []byte) []byte {
	b = e.AppendTag(b, wire.Semantic, wire.Hex)
	b = e.AppendStringBytes(b, wire.Bytes, x.b)
	return b
}
