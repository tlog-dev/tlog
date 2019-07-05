package tlog

import (
	"bytes"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/nikandfor/json"
	"github.com/stretchr/testify/assert"
)

func TestTlogParallel(t *testing.T) {
	const M = 10
	const N = 2

	var buf bytes.Buffer

	defer func(l *Logger) {
		DefaultLogger = l
	}(DefaultLogger)
	DefaultLogger = NewLogger(NewConsoleWriter(&buf, LstdFlags))

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

func TestVerbosity(t *testing.T) {
	defer func(old func() time.Time) {
		now = old
	}(now)
	tm := time.Date(2019, time.July, 5, 23, 49, 40, 0, time.Local)
	now = func() time.Time {
		tm = tm.Add(time.Second)
		return tm
	}

	var buf bytes.Buffer

	DefaultLogger = NewLogger(NewConsoleWriter(&buf, LstdFlags))

	Printf("unconditional message")
	V(LevError).Printf("Error level (enabled)")
	V(LevDebug).Printf("Debug level (disabled)")

	if l := V(LevInfo); l != nil {
		p := 10 + 20 // complex calculations
		l.Printf("conditional calculations (enabled): %v", p)
	}

	if l := V(LevTrace); l != nil {
		p := 10 + 50 // complex calculations
		l.Printf("conditional calculations (disabled): %v", p)
		assert.Fail(t, "should not be here")
	}

	assert.Equal(t, `2019/07/05_23:49:41  unconditional message
2019/07/05_23:49:42  Error level (enabled)
2019/07/05_23:49:43  conditional calculations (enabled): 30
`, string(buf.Bytes()))
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
