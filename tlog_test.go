package tlog

import (
	"bytes"
	"io/ioutil"
	"log"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type testt struct{}

func (t *testt) Func(l *Logger) {
	l.Printf("pointer receiver")
}

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

	ll.Del("key")
	assert.ElementsMatch(t, Labels{"=key", "key2=val2"}, ll)

	ll.Del("key2")
	assert.ElementsMatch(t, Labels{"=key", "=key2"}, ll)

	ll.Set("key", "value")
	assert.ElementsMatch(t, Labels{"key=value", "=key2"}, ll)

	ll.Set("key2", "")
	assert.ElementsMatch(t, Labels{"key=value", "key2"}, ll)

	ll.Merge(Labels{"", "key2=val2", "=key"})
	assert.ElementsMatch(t, Labels{"=key", "key2=val2"}, ll)

	ll.Del("key")
	assert.ElementsMatch(t, Labels{"=key", "key2=val2"}, ll)

	ll.Set("flag", "")

	v, ok := ll.Get("key2")
	assert.True(t, ok)
	assert.Equal(t, "val2", v)

	v, ok = ll.Get("key")
	assert.False(t, ok)

	v, ok = ll.Get("flag")
	assert.True(t, ok)
	assert.Equal(t, "", v)
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

	DefaultLogger.SetLogLevel(7)

	if l := V(LevTrace); l != nil {
		p := 10 + 60 // complex calculations
		l.Printf("trace: %v", p)
	}

	assert.Equal(t, `2019/07/05_23:49:41  unconditional message
2019/07/05_23:49:42  Error level (enabled)
2019/07/05_23:49:43  conditional calculations (enabled): 30
2019/07/05_23:49:44  trace: 70
`, string(buf.Bytes()))

	(*Logger)(nil).SetLogLevel(-1)
}

func TestDumpLabelsWithDefault(t *testing.T) {
	assert.Equal(t, Labels{"a", "b", "c"}, FillLabelsWithDefaults("a", "b", "c"))

	assert.Equal(t, Labels{"a=b", "f"}, FillLabelsWithDefaults("a=b", "f"))

	assert.Equal(t, Labels{"_hostname=myhost", "_pid=mypid"}, FillLabelsWithDefaults("_hostname=myhost", "_pid=mypid"))

	ll := FillLabelsWithDefaults("_hostname", "_pid")

	re := regexp.MustCompile(`_hostname=[\w-]+`)
	assert.True(t, re.MatchString(ll[0]), "%s is not %s ", ll[0], re)

	re = regexp.MustCompile(`_pid=\d+`)
	assert.True(t, re.MatchString(ll[1]), "%s is not %s ", ll[1], re)
}

func TestSpan(t *testing.T) {
	defer func(l *Logger) {
		DefaultLogger = l
	}(DefaultLogger)
	DefaultLogger = NewLogger(NewConsoleWriter(ioutil.Discard, LstdFlags))

	tr := Start()
	assert.NotNil(t, tr)

	tr2 := Spawn(tr.SafeID())
	assert.NotNil(t, tr2)

	DefaultLogger = nil

	tr = Start()
	assert.Nil(t, tr)

	tr2 = Spawn(tr.SafeID())
	assert.Nil(t, tr2)

	tr = DefaultLogger.Start()
	assert.Nil(t, tr)

	tr2 = DefaultLogger.Spawn(tr.SafeID())
	assert.Nil(t, tr2)

	assert.NotPanics(t, func() {
		tr.Printf("msg")

		tr2.Finish()
	})
}

//line tlog_test.go:179
func TestConsoleWriter(t *testing.T) {
	defer func(old func() time.Time) {
		now = old
	}(now)
	defer func(l *Logger) {
		DefaultLogger = l
	}(DefaultLogger)

	tm := time.Date(2019, time.July, 6, 9, 06, 19, 100000, time.UTC)
	now = func() time.Time {
		tm = tm.Add(time.Second)
		return tm
	}

	var buf bytes.Buffer
	DefaultLogger = NewLogger(NewConsoleWriter(&buf, LdetFlags|Lspans|LUTC|Lfuncname))

	DefaultLogger.Labels(Labels{"a=b", "f"})

	tr := Start()

	tr.Printf("msg")

	tr1 := Spawn(tr.SafeID())

	u := uint64(0xff << 56)
	tr1.Flags |= FlagError | 0x100 | int(u)

	tr1.Finish()

	tr.Finish()

	DefaultLogger.Writer.(*ConsoleWriter).f |= Lmilliseconds | Ltypefunc

	tr.Printf("message after finish with milliseconds")

	DefaultLogger.Writer.(*ConsoleWriter).f &^= Lshortfile | Ltypefunc

	(&testt{}).Func(DefaultLogger)

	DefaultLogger.Writer.(*ConsoleWriter).f &^= Lspans

	tr.Finish()

	assert.Equal(t, `2019/07/06_09:06:20.000100  tlog_test.go:196      TestConsoleWriter  Labels: ["a=b" "f"]
2019/07/06_09:06:21.000100  tlog_test.go:179      TestConsoleWriter  Span 2f0d18fb750b2d4a par ________________ started
2019/07/06_09:06:22.000100  tlog_test.go:200      TestConsoleWriter  msg
2019/07/06_09:06:23.000100  tlog_test.go:179      TestConsoleWriter  Span 250db1e09ea748af par 2f0d18fb750b2d4a started
2019/07/06_09:06:23.000100  tlog_test.go:179      TestConsoleWriter  Span 250db1e09ea748af par 2f0d18fb750b2d4a finished - elapsed 1000.00ms Flags ff00000000000101
2019/07/06_09:06:21.000100  tlog_test.go:179      TestConsoleWriter  Span 2f0d18fb750b2d4a par ________________ finished - elapsed 4000.00ms
2019/07/06_09:06:26.000  tlog_test.go:213      tlog.TestConsoleWriter  message after finish with milliseconds
2019/07/06_09:06:27.000  Func          pointer receiver
`, buf.String())
}

//line :239
func TestTeeWriter(t *testing.T) {
	var buf1, buf2 bytes.Buffer

	w1 := NewJSONWriter(&buf1)
	w2 := NewJSONWriter(&buf2)

	w := NewTeeWriter(w1, w2)

	w.Labels(Labels{"a=b", "f"})
	w.Message(Message{Format: "msg"}, nil)
	w.SpanStarted(&Span{ID: 100, Started: time.Date(2019, 7, 6, 10, 18, 32, 0, time.UTC)})
	w.SpanFinished(&Span{ID: 100, Elapsed: time.Second})

	assert.Equal(t, `{"L":["a=b","f"]}
{"l":{"pc":0,"f":"","l":0,"n":"."}}
{"m":{"l":0,"t":0,"m":"msg"}}
{"s":{"id":100,"l":0,"s":1562408312000000}}
{"f":{"id":100,"e":1000000}}
`, buf1.String())
	assert.Equal(t, buf1.String(), buf2.String())
}

func TestIDString(t *testing.T) {
	assert.Equal(t, "1234567890abcdef", ID(0x1234567890abcdef).String())
	assert.Equal(t, "________________", ID(0).String())
}

func TestMessageSpanID(t *testing.T) {
	m := Message{
		Args: []interface{}{
			ID(0x1234567890abcdef),
		},
	}
	assert.Equal(t, ID(0x1234567890abcdef), m.SpanID())

	m.Args = nil
	assert.Equal(t, ID(0), m.SpanID())
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
	l := NewLogger(NewJSONWriter(&buf))

	for i := 0; i < b.N; i++ {
		tr := l.Start()
		tr.Printf("message: %d", i) // 2 allocs here: new(int) and make([]interface{}, 1)
		tr.Finish()
	}
}
