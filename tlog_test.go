package tlog

import (
	"bytes"
	"log"
	"sync"
	"testing"

	"github.com/nikandfor/json"
	"github.com/stretchr/testify/assert"
)

func TestTlogParallel(t *testing.T) {
	const M = 10
	const N = 2

	var wg sync.WaitGroup
	wg.Add(M)
	for j := 0; j < M; j++ {
		go func(j int) {
			defer wg.Done()
			for i := 0; i < N; i++ {
				Printf("do j %d i %d", j, i)
			}
		}(j)
	}
	wg.Wait()
}

func TestLabels(t *testing.T) {
	var ll Labels

	ll.Set("key", "value")
	assert.ElementsMatch(t, Labels{"key=value"}, ll)

	ll.Set("key2", "val2")
	assert.ElementsMatch(t, Labels{"key=value", "key2=val2"}, ll)

	ll.Set("key", "pelupe")
	assert.ElementsMatch(t, Labels{"key=pelupe", "key2=val2"}, ll)

	v, ok := ll.Get("key2")
	assert.True(t, ok)
	assert.Equal(t, "val2", v)

	v, ok = ll.Get("key")
	assert.True(t, ok)
	assert.Equal(t, "pelupe", v)

	_, ok = ll.Get("pep")
	assert.False(t, ok)

	ll.Del("key")
	assert.ElementsMatch(t, Labels{"=key", "key2=val2"}, ll)

	ll.Del("key2")
	assert.ElementsMatch(t, Labels{"=key", "=key2"}, ll)

	ll.Set("key", "value")
	assert.ElementsMatch(t, Labels{"key=value", "=key2"}, ll)

	ll.Set("key2", "")
	assert.ElementsMatch(t, Labels{"key=value", "key2"}, ll)
}

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
