package tlog

import (
	"bytes"
	"io/ioutil"
	"log"
	"math/rand"
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

func (t *testt) testloc() Location {
	return Caller(0)
}

func (t *testt) testloc2() Location {
	return func() Location {
		return Caller(0)
	}()
}

func TestTlogParallel(t *testing.T) {
	const M = 10
	const N = 2

	var buf bytes.Buffer

	defer func(l *Logger) {
		DefaultLogger = l
	}(DefaultLogger)
	DefaultLogger = New(NewConsoleWriter(&buf, LstdFlags))

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

func TestPanicf(t *testing.T) {
	defer func(l *Logger) {
		DefaultLogger = l
	}(DefaultLogger)
	tm := time.Date(2019, time.July, 6, 19, 45, 25, 0, time.Local)

	now = func() time.Time {
		tm = tm.Add(time.Second)
		return tm
	}

	var buf bytes.Buffer
	DefaultLogger = New(NewConsoleWriter(&buf, LstdFlags))

	assert.Panics(t, func() {
		Panicf("panic! %v", 1)
	})

	assert.Panics(t, func() {
		DefaultLogger.Panicf("panic! %v", 2)
	})

	assert.Equal(t, `2019/07/06_19:45:26  panic! 1
2019/07/06_19:45:27  panic! 2
`, buf.String())
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

	DefaultLogger = New(NewConsoleWriter(&buf, LstdFlags))

	V("any_topic").Printf("All conditionals are disabled by default")

	SetFilter("topic1,tlog=topic3")

	Printf("unconditional message")
	V("topic1").Printf("topic1 message (enabled)")
	V("topic2").Printf("topic2 message (disabled)")

	if l := V("topic3"); l != nil {
		p := 10 + 20 // complex calculations
		l.Printf("conditional calculations (enabled): %v", p)
	}

	if l := V("topic4"); l != nil {
		p := 10 + 50 // complex calculations
		l.Printf("conditional calculations (disabled): %v", p)
		assert.Fail(t, "should not be here")
	}

	DefaultLogger.SetFilter("topic1,tlog=TRACE")

	if l := V("TRACE"); l != nil {
		p := 10 + 60 // complex calculations
		l.Printf("TRACE: %v", p)
	}

	tr := V("topic1").Start()
	tr.Printf("traced msg")
	tr.Finish()

	assert.Equal(t, `2019/07/05_23:49:41  unconditional message
2019/07/05_23:49:42  topic1 message (enabled)
2019/07/05_23:49:43  conditional calculations (enabled): 30
2019/07/05_23:49:44  TRACE: 70
2019/07/05_23:49:46  traced msg
`, string(buf.Bytes()))

	(*Logger)(nil).V("a,b,c").Printf("nothing")
	(*Logger)(nil).SetFilter("a,b,c")
}

func TestSetFilter(t *testing.T) {
	const N = 100

	l := New(Discard{})

	var wg sync.WaitGroup
	wg.Add(4)

	go func() {
		defer wg.Done()

		for i := 0; i < N; i++ {
			l.SetFilter("topic,topic2")
		}
	}()

	go func() {
		defer wg.Done()

		for i := 0; i < N; i++ {
			l.SetFilter("topic,topic3")
		}
	}()

	go func() {
		defer wg.Done()

		for i := 0; i < N; i++ {
			l.SetFilter("")
		}
	}()

	go func() {
		defer wg.Done()

		for i := 0; i < N; i++ {
			l.V("topic")
		}
	}()

	wg.Wait()
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
	DefaultLogger = New(NewConsoleWriter(ioutil.Discard, LstdFlags))

	tr := Start()
	assert.NotNil(t, tr)

	tr2 := Spawn(tr.SafeID())
	assert.NotNil(t, tr2)

	tr = DefaultLogger.Start()
	assert.NotNil(t, tr)

	tr2 = DefaultLogger.Spawn(tr.SafeID())
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

func TestTeeWriter(t *testing.T) {
	var buf1, buf2 bytes.Buffer

	w1 := NewJSONWriter(&buf1)
	w2 := NewJSONWriter(&buf2)

	w := NewTeeWriter(w1, w2)

	w.Labels(Labels{"a=b", "f"})
	w.Message(Message{Format: "msg"}, nil)
	w.SpanStarted(&Span{ID: 100, Started: time.Date(2019, 7, 6, 10, 18, 32, 0, time.UTC)}, 0)
	w.SpanFinished(&Span{ID: 100}, time.Second)

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

func TestConsoleWriterAppendSegment(t *testing.T) {
	b := []byte("prefix     ")
	i := 7

	var w ConsoleWriter

	b, e := w.appendSegments(b, i, 20, "path/to/file.go", '/')
	assert.Equal(t, "prefix path/to/file.go     ", string(b[:i+20]), "%q", string(b))
	assert.Equal(t, 22, e)

	b, e = w.appendSegments(b, i, 12, "path/to/file.go", '/')
	assert.Equal(t, "prefix p/to/file.go", string(b[:i+12]), "%q", string(b))
	assert.Equal(t, 19, e)

	b, e = w.appendSegments(b, i, 11, "path/to/file.go", '/')
	assert.Equal(t, "prefix p/t/file.go", string(b[:i+11]), "%q", string(b))
	assert.Equal(t, 18, e)

	b, e = w.appendSegments(b, i, 10, "path/to/file.go", '/')
	assert.Equal(t, "prefix p/t/file.g", string(b[:i+10]), "%q", string(b))
	assert.Equal(t, 17, e)

	b, e = w.appendSegments(b, i, 9, "path/to/file.go", '/')
	assert.Equal(t, "prefix p/t/file.", string(b[:i+9]), "%q", string(b))
	assert.Equal(t, 16, e)
}

func TestConsoleWriterBuildHeader(t *testing.T) {
	var w ConsoleWriter

	tm := time.Date(2019, 7, 7, 8, 19, 30, 100200300, time.UTC)
	loc := Caller(-1)

	w.f = Ldate | Ltime | Lmilliseconds | LUTC
	w.buildHeader(tm, loc)
	assert.Equal(t, "2019/07/07_08:19:30.100  ", string(w.buf))

	w.f = Ldate | Ltime | Lmicroseconds | LUTC
	w.buildHeader(tm, loc)
	assert.Equal(t, "2019/07/07_08:19:30.100200  ", string(w.buf))

	w.f = Llongfile
	w.buildHeader(tm, loc)
	ok, err := regexp.Match("(github.com/nikandfor/tlog/)?location.go:14  ", w.buf)
	assert.NoError(t, err)
	assert.True(t, ok)

	w.f = Lshortfile
	w.shortfile = 20
	w.buildHeader(tm, loc)
	assert.Equal(t, "location.go:14        ", string(w.buf))

	w.f = Lshortfile
	w.shortfile = 10
	w.buildHeader(tm, loc)
	assert.Equal(t, "locatio:14  ", string(w.buf))

	w.f = Lfuncname
	w.funcname = 10
	w.buildHeader(tm, loc)
	assert.Equal(t, "Caller      ", string(w.buf))

	w.f = Lfuncname
	w.funcname = 4
	w.buildHeader(tm, loc)
	assert.Equal(t, "Call  ", string(w.buf))

	w.f = Lfuncname
	w.funcname = 15
	w.buildHeader(tm, (&testt{}).testloc2())
	assert.Equal(t, "testloc2.func1   ", string(w.buf))

	w.f = Lfuncname
	w.funcname = 12
	w.buildHeader(tm, (&testt{}).testloc2())
	assert.Equal(t, "testloc2.fu1  ", string(w.buf))

	w.f = Ltypefunc
	w.buildHeader(tm, loc)
	assert.Equal(t, "tlog.Caller  ", string(w.buf))

	w.buildHeader(tm, (&testt{}).testloc2())
	assert.Equal(t, "tlog.(*testt).testloc2.func1  ", string(w.buf))
}

//line tlog_test.go:351
func TestConsoleWriterSpans(t *testing.T) {
	tm := time.Date(2019, time.July, 7, 16, 31, 10, 0, time.Local)
	now = func() time.Time {
		tm = tm.Add(time.Second)
		return tm
	}
	rnd = rand.New(rand.NewSource(0))

	w := NewConsoleWriter(ioutil.Discard, LdetFlags|Lspans|Lmessagespan)
	l := New(w)

	l.Labels(Labels{"a=b", "f"})

	assert.Equal(t, `2019/07/07_16:31:11.000000  tlog_test.go:362      Labels: ["a=b" "f"]`+"\n", string(w.buf))

	tr := l.Start()

	assert.Equal(t, `2019/07/07_16:31:12.000000  tlog_test.go:351      Span 78fc2ffac2fd9401 par ________________ started`+"\n", string(w.buf))

	tr1 := l.Spawn(tr.SafeID())

	assert.Equal(t, `2019/07/07_16:31:13.000000  tlog_test.go:351      Span 1f5b0412ffd341c0 par 78fc2ffac2fd9401 started`+"\n", string(w.buf))

	tr1.Printf("message")

	assert.Equal(t, `2019/07/07_16:31:14.000000  tlog_test.go:374      Span 1f5b0412ffd341c0 message`+"\n", string(w.buf))

	tr1.Finish()

	assert.Equal(t, `2019/07/07_16:31:15.000000  .:0                   Span 1f5b0412ffd341c0 par 78fc2ffac2fd9401 finished - elapsed 2000.00ms`+"\n", string(w.buf))

	tr.Flags |= FlagError | 0x100

	tr.Finish()

	assert.Equal(t, `2019/07/07_16:31:16.000000  .:0                   Span 78fc2ffac2fd9401 par ________________ finished - elapsed 4000.00ms Flags 101`+"\n", string(w.buf))
}

//line /path/to/github.com/nikandfor/tlog/tlog_test.go:389
func TestJSONWriterSpans(t *testing.T) {
	tm := time.Date(2019, time.July, 7, 16, 31, 10, 0, time.UTC)
	now = func() time.Time {
		tm = tm.Add(time.Second)
		return tm
	}
	rnd = rand.New(rand.NewSource(0))

	var buf bytes.Buffer
	w := NewJSONWriter(&buf)
	l := New(w)

	l.Labels(Labels{"a=b", "f"})

	tr := l.Start()

	tr1 := l.Spawn(tr.SafeID())

	tr1.Printf("message")

	tr1.Finish()

	tr.Flags |= FlagError | 0x100

	tr.Finish()

	re := `{"L":\["a=b","f"\]}
{"l":{"pc":\d+,"f":"github.com/nikandfor/tlog/tlog_test.go","l":389,"n":"tlog.TestJSONWriterSpans"}}
{"s":{"id":8717895732742165505,"l":\d+,"s":1562517071000000}}
{"s":{"id":2259404117704393152,"p":8717895732742165505,"l":\d+,"s":1562517072000000}}
{"l":{"pc":\d+,"f":"github.com/nikandfor/tlog/tlog_test.go","l":407,"n":"tlog.TestJSONWriterSpans"}}
{"m":{"l":\d+,"t":1000000,"m":"message","s":2259404117704393152}}
{"f":{"id":2259404117704393152,"e":2000000}}
{"f":{"id":8717895732742165505,"e":4000000,"F":257}}
`
	ok, err := regexp.Match(re, buf.Bytes())
	assert.NoError(t, err)
	assert.True(t, ok, "expected:\n%vactual:\n%v", re, buf.String())
}

func BenchmarkLogLoggerStd(b *testing.B) {
	l := log.New(ioutil.Discard, "", log.LstdFlags)

	for i := 0; i < b.N; i++ {
		l.Printf("message: %d", i)
	}
}

func BenchmarkTlogConsoleLoggerStd(b *testing.B) {
	b.ReportAllocs()

	l := New(NewConsoleWriter(ioutil.Discard, LstdFlags))

	for i := 0; i < b.N; i++ {
		l.Printf("message: %d", i)
	}
}

func BenchmarkLogLoggerDetailed(b *testing.B) {
	b.ReportAllocs()

	l := log.New(ioutil.Discard, "", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile)

	for i := 0; i < b.N; i++ {
		l.Printf("message: %d", i)
	}
}

func BenchmarkTlogConsoleDetailed(b *testing.B) {
	b.ReportAllocs()

	l := New(NewConsoleWriter(ioutil.Discard, LdetFlags))

	for i := 0; i < b.N; i++ {
		l.Printf("message: %d", i) // 2 allocs here: new(int) and make([]interface{}, 1)
	}
}

func BenchmarkTlogTracesConsoleFull(b *testing.B) {
	b.ReportAllocs()

	l := New(NewConsoleWriter(ioutil.Discard, LdetFlags|Lspans))

	for i := 0; i < b.N; i++ {
		tr := l.Start()
		tr.Printf("message: %d", i) // 2 allocs here: new(int) and make([]interface{}, 1)
		tr.Finish()
	}
}

func BenchmarkTlogTracesJSONFull(b *testing.B) {
	b.ReportAllocs()

	l := New(NewJSONWriter(ioutil.Discard))

	for i := 0; i < b.N; i++ {
		tr := l.Start()
		tr.Printf("message: %d", i) // 2 allocs here: new(int) and make([]interface{}, 1)
		tr.Finish()
	}
}
