package tlog

import (
	"bytes"
	"fmt"
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

func TestPrintRaw(t *testing.T) {
	defer func(l *Logger) {
		DefaultLogger = l
	}(DefaultLogger)

	var buf bytes.Buffer
	DefaultLogger = New(NewConsoleWriter(&buf, 0))

	PrintRaw(0, []byte("raw message 1"))
	DefaultLogger.PrintRaw(0, []byte("raw message 2"))

	tr := Start()
	tr.PrintRaw(0, []byte("raw message 3"))
	tr.Finish()

	tr = Span{}
	tr.PrintRaw(0, []byte("raw message 4"))

	assert.Equal(t, `raw message 1
raw message 2
raw message 3
`, buf.String())
}

func TestPrintfDepth(t *testing.T) {
	defer func(l *Logger) {
		DefaultLogger = l
	}(DefaultLogger)

	var buf bytes.Buffer
	DefaultLogger = New(NewConsoleWriter(&buf, 0))

	PrintfDepth(0, "message %d", 1)
	DefaultLogger.PrintfDepth(0, "message %d", 2)

	tr := Start()
	tr.PrintfDepth(0, "message %d", 3)
	tr.Finish()

	tr = Span{}
	tr.PrintfDepth(0, "message %d", 4)

	assert.Equal(t, `message 1
message 2
message 3
`, buf.String())
}

func TestWrite(t *testing.T) {
	defer func(l *Logger) {
		DefaultLogger = l
	}(DefaultLogger)

	var buf bytes.Buffer
	DefaultLogger = New(NewConsoleWriter(&buf, 0))

	n, err := DefaultLogger.Write([]byte("raw message 2"))
	assert.NoError(t, err)
	assert.Equal(t, 13, n)

	tr := Start()
	n, err = tr.Write([]byte("raw message 3"))
	assert.NoError(t, err)
	assert.Equal(t, 13, n)
	tr.Finish()

	tr = Span{}
	n, err = tr.Write([]byte("raw message 4"))
	assert.NoError(t, err)
	assert.Equal(t, 13, n)

	assert.Equal(t, `raw message 2
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

	assert.Equal(t, "", (*Logger)(nil).Filter())
	assert.Equal(t, "", NamedFilter(""))

	var buf bytes.Buffer

	DefaultLogger = New(NewConsoleWriter(&buf, Lnone))

	V("any_topic").Printf("All conditionals are disabled by default")

	SetFilter("topic1,tlog=topic3")

	assert.Equal(t, "topic1,tlog=topic3", Filter())

	SetNamedFilter("", "topic1,tlog=topic3")

	assert.Equal(t, "topic1,tlog=topic3", Filter())

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
	(*Logger)(nil).SetNamedFilter("", "a,b,c")

	DefaultLogger = nil
	V("a").Printf("none")
}

func TestVerbosity2(t *testing.T) {
	defer func(old func() time.Time) {
		now = old
	}(now)
	tm := time.Date(2019, time.July, 5, 23, 49, 40, 0, time.Local)
	now = func() time.Time {
		tm = tm.Add(time.Second)
		return tm
	}
	randID = testRandID()

	var buf0, buf1, buf2 bytes.Buffer

	l := New(
		NewConsoleWriter(&buf0, Lspans),
		NewNamedWriter("a", "a", NewConsoleWriter(&buf1, Lspans)),
		NewNamedWriter("b", "b", NewConsoleWriter(&buf2, Lspans)))

	l.Printf("unconditional")

	l.V("a").Printf("a only")

	l.V("b").Printf("b only")

	l.V("c").Printf("nowhere")

	tr := l.Start()
	tr.V("a").Printf("a trace")
	tr.Finish()

	l.SetNamedFilter("b", "b,c")

	assert.Equal(t, "b,c", l.NamedFilter("b"))
	assert.Equal(t, "", l.Filter())
	assert.Equal(t, "", (*Logger)(nil).Filter())

	l.V("c").Printf("b3")

	l.SetFilter("q")

	l.Printf("unconditional 2")
	l.V("q").Printf("conditional 1")
	l.V("w").Printf("conditional 2")

	assert.Equal(t, `unconditional
0194fdc2fa2ffcc0  Span started
0194fdc2fa2ffcc0  Span finished - elapsed 2000.00ms
unconditional 2
conditional 1
`, buf0.String())

	assert.Equal(t, `unconditional
a only
0194fdc2fa2ffcc0  Span started
a trace
0194fdc2fa2ffcc0  Span finished - elapsed 2000.00ms
unconditional 2
`, buf1.String())

	assert.Equal(t, `unconditional
b only
0194fdc2fa2ffcc0  Span started
0194fdc2fa2ffcc0  Span finished - elapsed 2000.00ms
b3
unconditional 2
`, buf2.String())
}

func TestVerbosity3(t *testing.T) {
	defer func(old func() time.Time) {
		now = old
	}(now)
	tm := time.Date(2019, time.July, 5, 23, 49, 40, 0, time.Local)
	now = func() time.Time {
		tm = tm.Add(time.Second)
		return tm
	}
	randID = testRandID()

	var buf0, buf1, buf2 bytes.Buffer

	l := New(
		NewConsoleWriter(&buf0, Lspans),
		NewNamedDumper("a", "a", NewConsoleWriter(&buf1, Lspans)),
		NewNamedDumper("b", "b", NewConsoleWriter(&buf2, Lspans)))

	l.Printf("unconditional")

	l.V("a").Printf("a only")

	l.V("b").Printf("b only")

	l.V("c").Printf("nowhere")

	tr := l.Start()
	tr.V("a").Printf("a trace")
	tr.Finish()

	l.SetNamedFilter("b", "b,c")

	assert.Equal(t, "b,c", l.NamedFilter("b"))
	assert.Equal(t, "", l.Filter())
	assert.Equal(t, "", (*Logger)(nil).Filter())

	l.V("c").Printf("b3")

	l.SetFilter("q")

	l.Printf("unconditional 2")
	l.V("q").Printf("conditional 1")
	l.V("w").Printf("conditional 2")

	assert.Equal(t, `unconditional
0194fdc2fa2ffcc0  Span started
0194fdc2fa2ffcc0  Span finished - elapsed 2000.00ms
unconditional 2
conditional 1
`, buf0.String())

	assert.Equal(t, `a only
a trace
`, buf1.String())

	assert.Equal(t, `b only
b3
`, buf2.String())
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
	assert.NotZero(t, tr)

	tr2 := Spawn(tr.ID)
	assert.NotZero(t, tr2)

	tr2 = SpawnOrStart(z)
	assert.NotZero(t, tr2)

	tr = DefaultLogger.Start()
	assert.NotZero(t, tr)

	tr2 = DefaultLogger.Spawn(tr.ID)
	assert.NotZero(t, tr2)

	DefaultLogger = nil

	tr = Start()
	assert.Zero(t, tr)

	tr2 = Spawn(tr.ID)
	assert.Zero(t, tr2)

	tr2 = SpawnOrStart(tr.ID)
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

func TestIDString(t *testing.T) {
	assert.Equal(t, "1234567890abcdef", ID{0x12, 0x34, 0x56, 0x78, 0x90, 0xab, 0xcd, 0xef, 0x11, 0x22}.String())
	assert.Equal(t, "________________", ID{}.String())
	assert.Equal(t, "1234567890abcdef1122000000000000", ID{0x12, 0x34, 0x56, 0x78, 0x90, 0xab, 0xcd, 0xef, 0x11, 0x22}.FullString())
	assert.Equal(t, "________________________________", ID{}.FullString())

	assert.Equal(t, "1234567890abcdef", fmt.Sprintf("%v", ID{0x12, 0x34, 0x56, 0x78, 0x90, 0xab, 0xcd, 0xef, 0x11, 0x22}))
	assert.Equal(t, "1234567890abcdef1122000000000000", fmt.Sprintf("%+v", ID{0x12, 0x34, 0x56, 0x78, 0x90, 0xab, 0xcd, 0xef, 0x11, 0x22}))
}

func TestIDFrom(t *testing.T) {
	id := ID{1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 0xa, 0xb}

	res, err := IDFromString(id.FullString())
	assert.NoError(t, err)
	assert.Equal(t, id, res)

	res, err = IDFromBytes(id[:])
	assert.NoError(t, err)
	assert.Equal(t, id, res)

	res, err = IDFromString(fmt.Sprintf("%8x", id))
	assert.Equal(t, TooShortIDError{N: 4}, err)
	assert.Equal(t, ID{1, 2, 3, 4}, res)

	res, err = IDFromBytes(id[:4])
	assert.Equal(t, TooShortIDError{N: 4}, err)
	assert.Equal(t, ID{1, 2, 3, 4}, res)

	res, err = IDFromString("010203046q")
	assert.EqualError(t, err, "encoding/hex: invalid byte: U+0071 'q'")
	assert.Equal(t, ID{1, 2, 3, 4}, res)

	res, err = IDFromString(ID{}.FullString())
	assert.NoError(t, err)
	assert.Equal(t, ID{}, res)
}

func TestIDFromMustShould(t *testing.T) {
	id := ID{1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 0xa, 0xb}

	// strings
	// should
	res := ShouldIDFromString(id.FullString())
	assert.Equal(t, id, res)

	res = ShouldIDFromString(id.String())
	assert.Equal(t, ID{1, 2, 3, 4, 5, 6, 7, 8}, res)

	res = ShouldIDFromString(ID{}.FullString())
	assert.Equal(t, z, res)

	res = ShouldIDFromString(ID{}.String())
	assert.Equal(t, z, res)

	// must
	res = MustIDFromString(id.FullString())
	assert.Equal(t, id, res)

	assert.Panics(t, func() { MustIDFromString(id.String()) })

	assert.Panics(t, func() { MustIDFromString("1234567") })

	res = MustIDFromString(ID{}.FullString())
	assert.Equal(t, z, res)

	assert.Panics(t, func() { MustIDFromString(ID{}.String()) })

	// bytes
	res = ShouldIDFromBytes(nil)
	assert.Equal(t, z, res)

	res = ShouldIDFromBytes(id[:])
	assert.Equal(t, id, res)

	assert.Panics(t, func() { MustIDFromBytes([]byte{1, 2, 3, 4, 5}) })

	res = MustIDFromBytes(id[:])
	assert.Equal(t, id, res)
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

	assert.Equal(t, `2019/07/07_16:31:11.000  ________________  Labels: ["a=b" "f"]`+"\n", string(w.buf))

	tr := l.Start()

	assert.Equal(t, "2019/07/07_16:31:12.000  0194fdc2fa2ffcc0  Span started\n", string(w.buf))

	tr1 := l.Spawn(tr.ID)

	assert.Equal(t, "2019/07/07_16:31:13.000  6e4ff95ff662a5ee  Span spawned from 0194fdc2fa2ffcc0\n", string(w.buf))

	tr1.Printf("message")

	assert.Equal(t, "2019/07/07_16:31:14.000  6e4ff95ff662a5ee  message\n", string(w.buf))

	tr1.Finish()

	assert.Equal(t, "2019/07/07_16:31:15.000  6e4ff95ff662a5ee  Span finished - elapsed 2000.00ms\n", string(w.buf))

	tr.Finish()

	assert.Equal(t, "2019/07/07_16:31:16.000  0194fdc2fa2ffcc0  Span finished - elapsed 4000.00ms\n", string(w.buf))

	l.Printf("not traced message")

	assert.Equal(t, "2019/07/07_16:31:17.000  ________________  not traced message\n", string(w.buf))
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
{"l":{"p":\d+,"f":"[\w.-/]*tlog_test.go","l":\d+,"n":"github.com/nikandfor/tlog.TestJSONWriterSpans"}}
{"s":{"i":"0194fdc2fa2ffcc041d3ff12045b73c8","l":\d+,"s":24414329234375000}}
{"s":{"i":"6e4ff95ff662a5eee82abdf44a2d0b75","p":"0194fdc2fa2ffcc041d3ff12045b73c8","l":\d+,"s":24414329250000000}}
{"l":{"p":\d+,"f":"[\w.-/]*tlog_test.go","l":\d+,"n":"github.com/nikandfor/tlog.TestJSONWriterSpans"}}
{"m":{"l":\d+,"t":15625000,"m":"message","s":"6e4ff95ff662a5eee82abdf44a2d0b75"}}
{"f":{"i":"6e4ff95ff662a5eee82abdf44a2d0b75","e":31250000}}
{"f":{"i":"0194fdc2fa2ffcc041d3ff12045b73c8","e":62500000}}
`

	ok, err := regexp.Match(re, buf.Bytes())
	assert.NoError(t, err)
	assert.True(t, ok, "expected:\n%vactual:\n%v", re, buf.String())
}

func TestAppendWriter(t *testing.T) {
	l := New(NewNamedWriter("a", "a", Discard{}), NewNamedWriter("b", "b", Discard{}))

	l.AppendWriter(NewNamedWriter("b", "b", Discard{}))

	assert.Len(t, l.ws, 2)

	assert.Panics(t, func() {
		l.AppendWriter(l)
	})

	assert.Panics(t, func() {
		l.AppendWriter("qwe")
	})

	assert.Panics(t, func() {
		l.AppendWriter("qwe", NewNamedWriter("", "", Discard{}))
	})

	l.AppendWriter("c", Discard{})

	assert.Len(t, l.ws, 3)
}

func TestCoverUncovered(t *testing.T) {
	defer func(l *Logger) {
		DefaultLogger = l
	}(DefaultLogger)

	var buf bytes.Buffer
	DefaultLogger = New(NewJSONWriter(&buf))

	SetLabels(Labels{"a", "q"})

	assert.Equal(t, `{"L":["a","q"]}`+"\n", buf.String())

	(*Logger)(nil).Labels(Labels{"a"})

	assert.Equal(t, "", DefaultLogger.NamedFilter("qq"))

	assert.Equal(t, "too short id: 7, wanted 32", TooShortIDError{N: 7}.Error())

	assert.Equal(t, "1", fmt.Sprintf("%01v", ID{0x12, 0x34, 0x56}))

	b := make([]byte, 8)
	ID{0xaa, 0xbb, 0xcc, 0x44, 0x55}.FormatTo(b, 'X')
	assert.Equal(t, "AABBCC44", string(b))

	ID{0xaa, 0xbb, 0x44, 0x55}.FormatQuotedTo(b, 'x')
	assert.Equal(t, `"aabb44"`, string(b))

	assert.Panics(t, func() { ID{}.FormatQuotedTo([]byte{}, 'x') })

	id := stdRandID()
	assert.NotZero(t, id)
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

func BenchmarkTlogTracesConsoleDetailed(b *testing.B) {
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
		tr.PrintRaw(0, buf)
		tr.Finish()
	}
}

func BenchmarkTlogTracesProtoWrite(b *testing.B) {
	b.ReportAllocs()

	l := New(NewProtoWriter(ioutil.Discard))

	for i := 0; i < b.N; i++ {
		tr := l.Start()
		fmt.Fprintf(tr, "message %d", i)
		tr.Finish()
	}
}

func BenchmarkIDFormat(b *testing.B) {
	b.ReportAllocs()

	id := ID{1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 0xa, 0xb, 0xc, 0xd, 0xe, 0xf}

	for i := 0; i < b.N; i++ {
		_ = fmt.Sprintf("%+x", id)
	}
}

func BenchmarkIDFormatTo(b *testing.B) {
	b.ReportAllocs()

	var buf [40]byte
	id := ID{1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 0xa, 0xb, 0xc, 0xd, 0xe, 0xf}

	for i := 0; i < b.N; i++ {
		id.FormatTo(buf[:], 'x')
	}
}
