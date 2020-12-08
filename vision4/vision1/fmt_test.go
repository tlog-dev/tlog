package tlog

import (
	"bytes"
	"fmt"
	"testing"
)

func BenchmarkFmtFprintf(b *testing.B) {
	b.ReportAllocs()

	var buf bytes.Buffer

	for i := 0; i < b.N; i++ {
		buf.Reset()

		fmt.Fprintf(&buf, "message %v %v %v", 1, "string", 3.12)
	}
}

func BenchmarkFmtAppendPrintf(b *testing.B) {
	b.ReportAllocs()

	var buf []byte

	for i := 0; i < b.N; i++ {
		buf = AppendPrintf(buf[:0], "message %v %v %v", 1, "string", 3.12)
	}
}
