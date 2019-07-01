package tlog

import (
	"bytes"
	"log"
	"testing"

	"github.com/nikandfor/json"
)

func BenchmarkLogLoggerStd(b *testing.B) {
	b.ReportAllocs()

	var buf bytes.Buffer
	l := log.New(&buf, "", log.LstdFlags)

	for i := 0; i < b.N; i++ {
		l.Printf("message: %d", i)
	}
}

func BenchmarkTlogConsoleLoggerStd(b *testing.B) {
	b.ReportAllocs()

	var buf bytes.Buffer
	l := NewLogger(NewConsoleWriter(&buf, LstdFlags))

	for i := 0; i < b.N; i++ {
		l.Printf("message: %d", i)
	}
}

func BenchmarkLogLoggerDetailed(b *testing.B) {
	b.ReportAllocs()

	var buf bytes.Buffer
	l := log.New(&buf, "", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile)

	for i := 0; i < b.N; i++ {
		l.Printf("message: %d", i)
	}
}

func BenchmarkTlogConsoleDetailed(b *testing.B) {
	b.ReportAllocs()

	var buf bytes.Buffer
	l := NewLogger(NewConsoleWriter(&buf, LdetFlags))

	for i := 0; i < b.N; i++ {
		l.Printf("message: %d", i) // 2 allocs here: new(int) and make([]interface{}, 1)
	}
}

func BenchmarkTlogTracesConsoleFull(b *testing.B) {
	b.ReportAllocs()

	var buf bytes.Buffer
	l := NewLogger(NewConsoleWriter(&buf, LdetFlags|Lspans))

	for i := 0; i < b.N; i++ {
		tr := l.Start()
		tr.Printf("message: %d", i) // 2 allocs here: new(int) and make([]interface{}, 1)
		tr.Finish()
	}
}

func BenchmarkTlogTracesJSONFull(b *testing.B) {
	b.ReportAllocs()

	var buf bytes.Buffer
	jw := json.NewStreamWriter(&buf)
	l := NewLogger(NewJSONWriter(jw))

	for i := 0; i < b.N; i++ {
		tr := l.Start()
		tr.Printf("message: %d", i) // 2 allocs here: new(int) and make([]interface{}, 1)
		tr.Finish()
	}
}
