package tlog

import (
	"io"
	"testing"

	"github.com/nikandfor/tlog/low"
)

func TestHex(t *testing.T) {
	var b low.Buf

	l := New(NewConsoleWriter(&b, 0))

	var x uint64 = 10

	l.Printw("hex", "hex", Hex(x), "hex_any", HexAny{x})

	t.Logf("%s", b)
}

func BenchmarkHex(b *testing.B) {
	b.ReportAllocs()

	l := New(io.Discard)

	var x uint64 = 10

	for i := 0; i < b.N; i++ {
		l.Printw("message", "hex", Hex(x))
	}
}

func BenchmarkHexAny(b *testing.B) {
	b.ReportAllocs()

	l := New(io.Discard)

	var x uint64 = 10

	for i := 0; i < b.N; i++ {
		l.Printw("message", "hex", HexAny{x})
	}
}
