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

func (t *testt) testloc2() Location {
	return func() Location {
		return Caller(0)
	}()
}

func TestTlogParallel(t *testing.T) {
	const M = 10
	const N = 2

	randID = testRandID()

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
				tr := Start()
				tr.Printf("trace j %d i %d", j, i)
				tr.Finish()
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

func TestRawMessage(t *testing.T) {
	defer func(l *Logger) {
		DefaultLogger = l
	}(DefaultLogger)

	var buf bytes.Buffer
	DefaultLogger = New(NewConsoleWriter(&buf, 0))

	PrintRaw([]byte("raw message 1"))
	DefaultLogger.PrintRaw([]byte("raw message 2"))

	tr := Start()
	tr.PrintRaw([]byte("raw message 3"))
	tr.Finish()

	tr = Span{}
	tr.PrintRaw([]byte("raw message 4"))

	assert.Equal(t, `raw message 1
raw message 2
raw message 3
`, buf.String())
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

	DefaultLogger = New(NewConsoleWriter(&buf, Lnone))

	V("any_topic").Printf("All conditionals are disabled by default")

	SetFilter("topic1,tlog=topic3")

	Printf("unconditional message")
	DefaultLogger.V("topic1").Printf("topic1 message (enabled)")
	DefaultLogger.V("topic2").Printf("topic2 message (disabled)")

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
	if tr.Valid() {
		tr.Printf("traced msg")
	}
	tr.V("topic2").Printf("trace conditioned message 1")
	if tr.V("TRACE").Valid() {
		tr.Printf("trace conditioned message 2")
	}
	tr.Finish()

	assert.Equal(t, `unconditional message
topic1 message (enabled)
conditional calculations (enabled): 30
TRACE: 70
traced msg
trace conditioned message 2
`, buf.String())

	(*Logger)(nil).V("a,b,c").Printf("nothing")
	(*Logger)(nil).SetFilter("a,b,c")

	SetLogLevel(0)
	assert.Nil(t, DefaultLogger.filter)

	SetLogLevel(1)
	assert.Equal(t, CriticalLevel, DefaultLogger.filter.f)

	SetLogLevel(2)
	assert.Equal(t, ErrorLevel, DefaultLogger.filter.f)

	SetLogLevel(3)
	assert.Equal(t, InfoLevel, DefaultLogger.filter.f)

	SetLogLevel(4)
	assert.Equal(t, DebugLevel, DefaultLogger.filter.f)

	SetLogLevel(5)
	assert.Equal(t, TraceLevel, DefaultLogger.filter.f)

	DefaultLogger = nil
	V("a").Printf("none")
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

func TestSpan(t *testing.T) {
	defer func(l *Logger) {
		DefaultLogger = l
	}(DefaultLogger)
	DefaultLogger = New(NewConsoleWriter(ioutil.Discard, LstdFlags))

	tr := Start()
	assert.NotNil(t, tr)

	tr2 := Spawn(tr.ID)
	assert.NotNil(t, tr2)

	tr = DefaultLogger.Start()
	assert.NotNil(t, tr)

	tr2 = DefaultLogger.Spawn(tr.ID)
	assert.NotNil(t, tr2)

	DefaultLogger = nil

	tr = Start()
	assert.Zero(t, tr)

	tr2 = Spawn(tr.ID)
	assert.Zero(t, tr2)

	tr = DefaultLogger.Start()
	assert.Zero(t, tr)

	tr2 = DefaultLogger.Spawn(tr.ID)
	assert.Zero(t, tr2)

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
	w.Message(Message{Format: "msg"}, Span{})
	w.SpanStarted(Span{ID: ID{100}, Started: time.Date(2019, 7, 6, 10, 18, 32, 0, time.UTC)}, z, 0)
	w.SpanFinished(Span{ID: ID{100}}, time.Second)

	assert.Equal(t, `{"L":["a=b","f"]}
{"l":{"pc":0,"f":"","l":0,"n":""}}
{"m":{"l":0,"t":0,"m":"msg"}}
{"s":{"id":"640000000000000000000000","l":0,"s":24412629875000000}}
{"f":{"id":"640000000000000000000000","e":15625000}}
`, buf1.String())
	assert.Equal(t, buf1.String(), buf2.String())
}

func TestIDString(t *testing.T) {
	assert.Equal(t, "1234567890abcdef", ID{0x12, 0x34, 0x56, 0x78, 0x90, 0xab, 0xcd, 0xef}.String())
	assert.Equal(t, "________________", ID{}.String())
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
	w.buildHeader(loc, tm)
	assert.Equal(t, "2019/07/07_08:19:30.100  ", string(w.buf))

	w.f = Ldate | Ltime | Lmicroseconds | LUTC
	w.buildHeader(loc, tm)
	assert.Equal(t, "2019/07/07_08:19:30.100200  ", string(w.buf))

	w.f = Llongfile
	w.buildHeader(loc, tm)
	ok, err := regexp.Match("(github.com/nikandfor/tlog/)?location.go:25  ", w.buf)
	assert.NoError(t, err)
	assert.True(t, ok)

	w.f = Lshortfile
	w.Shortfile = 20
	w.buildHeader(loc, tm)
	assert.Equal(t, "location.go:25        ", string(w.buf))

	w.f = Lshortfile
	w.Shortfile = 10
	w.buildHeader(loc, tm)
	assert.Equal(t, "locatio:25  ", string(w.buf))

	w.f = Lfuncname
	w.Funcname = 10
	w.buildHeader(loc, tm)
	assert.Equal(t, "Caller      ", string(w.buf))

	w.f = Lfuncname
	w.Funcname = 4
	w.buildHeader(loc, tm)
	assert.Equal(t, "Call  ", string(w.buf))

	w.f = Lfuncname
	w.Funcname = 15
	w.buildHeader((&testt{}).testloc2(), tm)
	assert.Equal(t, "testloc2.func1   ", string(w.buf))

	w.f = Lfuncname
	w.Funcname = 12
	w.buildHeader((&testt{}).testloc2(), tm)
	assert.Equal(t, "testloc2.fu1  ", string(w.buf))

	w.f = Ltypefunc
	w.buildHeader(loc, tm)
	assert.Equal(t, "tlog.Caller  ", string(w.buf))

	w.buildHeader((&testt{}).testloc2(), tm)
	assert.Equal(t, "tlog.(*testt).testloc2.func1  ", string(w.buf))
}

func TestConsoleWriterSpans(t *testing.T) {
	tm := time.Date(2019, time.July, 7, 16, 31, 10, 0, time.Local)
	now = func() time.Time {
		tm = tm.Add(time.Second)
		return tm
	}
	randID = testRandID()

	w := NewConsoleWriter(ioutil.Discard, Ldate|Ltime|Lmilliseconds|Lspans|Lmessagespan)
	l := New(w)

	l.Labels(Labels{"a=b", "f"})

	assert.Equal(t, `2019/07/07_16:31:11.000  Labels: ["a=b" "f"]`+"\n", string(w.buf))

	tr := l.Start()

	assert.Equal(t, `2019/07/07_16:31:12.000  Span 0194fdc2fa2ffcc0 par ________________ started`+"\n", string(w.buf))

	tr1 := l.Spawn(tr.ID)

	assert.Equal(t, `2019/07/07_16:31:13.000  Span 045b73c86e4ff95f par 0194fdc2fa2ffcc0 started`+"\n", string(w.buf))

	tr1.Printf("message")

	assert.Equal(t, `2019/07/07_16:31:14.000  Span 045b73c86e4ff95f message`+"\n", string(w.buf))

	tr1.Finish()

	assert.Equal(t, `2019/07/07_16:31:15.000  Span 045b73c86e4ff95f finished - elapsed 2000.00ms`+"\n", string(w.buf))

	tr.Finish()

	assert.Equal(t, `2019/07/07_16:31:16.000  Span 0194fdc2fa2ffcc0 finished - elapsed 4000.00ms`+"\n", string(w.buf))
}

func TestJSONWriterSpans(t *testing.T) {
	tm := time.Date(2019, time.July, 7, 16, 31, 10, 0, time.UTC)
	now = func() time.Time {
		tm = tm.Add(time.Second)
		return tm
	}
	randID = testRandID()

	var buf bytes.Buffer
	w := NewJSONWriter(&buf)
	l := New(w)

	l.Labels(Labels{"a=b", "f"})

	tr := l.Start()

	tr1 := l.Spawn(tr.ID)

	tr1.Printf("message")

	tr1.Finish()

	tr.Finish()

	re := `{"L":\["a=b","f"\]}
{"l":{"pc":\d+,"f":"[\w.-/]*tlog_test.go","l":\d+,"n":"github.com/nikandfor/tlog.TestJSONWriterSpans"}}
{"s":{"id":"0194fdc2fa2ffcc041d3ff12","l":\d+,"s":24414329234375000}}
{"s":{"id":"045b73c86e4ff95ff662a5ee","p":"0194fdc2fa2ffcc041d3ff12","l":\d+,"s":24414329250000000}}
{"l":{"pc":\d+,"f":"[\w.-/]*tlog_test.go","l":\d+,"n":"github.com/nikandfor/tlog.TestJSONWriterSpans"}}
{"m":{"l":\d+,"t":15625000,"m":"message","s":"045b73c86e4ff95ff662a5ee"}}
{"f":{"id":"045b73c86e4ff95ff662a5ee","e":31250000}}
{"f":{"id":"0194fdc2fa2ffcc041d3ff12","e":62500000}}
`

	ok, err := regexp.Match(re, buf.Bytes())
	assert.NoError(t, err)
	assert.True(t, ok, "expected:\n%vactual:\n%v", re, buf.String())
}

func BenchmarkLogLoggerStd(b *testing.B) {
	b.ReportAllocs()

	l := log.New(ioutil.Discard, "", log.LstdFlags)

	for i := 0; i < b.N; i++ {
		l.Printf("message: %d", i)
	}
}

func BenchmarkTlogConsoleLoggerStd(b *testing.B) {
	b.ReportAllocs()

	l := New(NewConsoleWriter(ioutil.Discard, LstdFlags))
	l.NoLocations = true

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

func BenchmarkTlogTracesConsole(b *testing.B) {
	b.ReportAllocs()

	l := New(NewConsoleWriter(ioutil.Discard, LdetFlags|Lspans))

	for i := 0; i < b.N; i++ {
		tr := l.Start()
		tr.Printf("message: %d", i) // 2 allocs here: new(int) and make([]interface{}, 1)
		tr.Finish()
	}
}

func BenchmarkTlogTracesJSON(b *testing.B) {
	b.ReportAllocs()

	l := New(NewJSONWriter(ioutil.Discard))

	for i := 0; i < b.N; i++ {
		tr := l.Start()
		tr.Printf("message: %d", i) // 2 allocs here: new(int) and make([]interface{}, 1)
		tr.Finish()
	}
}

func BenchmarkTlogTracesProto(b *testing.B) {
	b.ReportAllocs()

	l := New(NewProtoWriter(ioutil.Discard))

	for i := 0; i < b.N; i++ {
		tr := l.Start()
		tr.Printf("message: %d", i) // 2 allocs here: new(int) and make([]interface{}, 1)
		tr.Finish()
	}
}

func BenchmarkTlogTracesProtoPrintRaw(b *testing.B) {
	b.ReportAllocs()

	l := New(NewProtoWriter(ioutil.Discard))

	var buf = []byte("raw message") // reusable buffer

	for i := 0; i < b.N; i++ {
		tr := l.Start()
		// fill in buffer...
		tr.PrintRaw(buf)
		tr.Finish()
	}
}
