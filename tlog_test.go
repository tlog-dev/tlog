//nolint:gosec
package tlog

import (
	"bytes"
	crand "crypto/rand"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type testt struct{}

const (
	Parallel     = "Parallel"
	SingleThread = "SingleThread"
)

func testRandID() func() ID {
	rnd := rand.New(rand.NewSource(0))

	return func() (id ID) {
		for id == (ID{}) {
			_, _ = rnd.Read(id[:])
		}
		return
	}
}

func (t *testt) Func(l *Logger) {
	l.Printf("pointer receiver")
}

func (t *testt) testloc2() PC {
	return func() PC {
		return Caller(0)
	}()
}

func TestMessageSize(t *testing.T) {
	tp := reflect.TypeOf(Message{})

	t.Logf("Message size: %x (%[1]d), align %x (%[2]d)", tp.Size(), tp.Align())
}

func TestTlogParallel(t *testing.T) {
	const M = 10
	const N = 2

	defer func(l *Logger) {
		DefaultLogger = l
	}(DefaultLogger)

	var buf bytes.Buffer
	DefaultLogger = New(NewConsoleWriter(LockWriter(&buf), LstdFlags))
	DefaultLogger.randID = testRandID()

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
	tm := time.Date(2019, time.July, 6, 19, 45, 25, 0, time.Local)
	now = func() time.Time {
		tm = tm.Add(time.Second)
		return tm
	}

	var buf bytes.Buffer
	DefaultLogger = New(NewConsoleWriter(&buf, LstdFlags))
	DefaultLogger.randID = testRandID()

	assert.Panics(t, func() {
		Panicf("panic! %v", 1)
	})

	assert.Panics(t, func() {
		DefaultLogger.Panicf("panic! %v", 2)
	})

	assert.Equal(t, `2019-07-06_19:45:26  panic! 1
2019-07-06_19:45:27  panic! 2
`, buf.String())
}

func TestPrintBytes(t *testing.T) {
	defer func(l *Logger) {
		DefaultLogger = l
	}(DefaultLogger)

	var buf bytes.Buffer
	DefaultLogger = New(NewConsoleWriter(&buf, 0))

	PrintBytes(0, []byte("raw message 1"))
	DefaultLogger.PrintBytes(0, []byte("raw message 2"))

	tr := Start()
	tr.PrintBytes(0, []byte("raw message 3"))
	tr.Finish()

	tr = Span{}
	tr.PrintBytes(0, []byte("raw message 4"))

	assert.Equal(t, `raw message 1
raw message 2
raw message 3
`, buf.String())
}

func TestIOWriter(t *testing.T) {
	defer func(l *Logger) {
		DefaultLogger = l
	}(DefaultLogger)

	var buf bytes.Buffer
	DefaultLogger = New(NewConsoleWriter(&buf, 0))

	n, err := DefaultLogger.IOWriter(0).Write([]byte("raw message 2"))
	assert.NoError(t, err)
	assert.Equal(t, 13, n)

	tr := Start()
	n, err = tr.IOWriter(0).Write([]byte("raw message 3"))
	assert.NoError(t, err)
	assert.Equal(t, 13, n)
	tr.Finish()

	tr = Span{}
	n, err = tr.IOWriter(0).Write([]byte("raw message 4"))
	assert.NoError(t, err)
	assert.Equal(t, 13, n)

	assert.Equal(t, `raw message 2
raw message 3
`, buf.String())

	n, err = (*Logger)(nil).IOWriter(0).Write([]byte("123"))
	assert.NoError(t, err)
	assert.Equal(t, 3, n)
}

func TestPrintln(t *testing.T) {
	var buf bytes.Buffer
	DefaultLogger = New(NewConsoleWriter(&buf, 0))

	Println("message", 1)

	DefaultLogger.Println("message", 2)

	tr := Start()

	tr.Println("message", 3)

	tr.Finish()

	assert.Equal(t, `message 1
message 2
message 3
`, buf.String())
}

func TestPrintw(t *testing.T) {
	var buf bytes.Buffer

	cw := NewConsoleWriter(&buf, 0)

	DefaultLogger = New(cw)
	DefaultLogger.randID = testRandID()

	cfg := DefaultStructuredConfig.Copy()
	cfg.MessageWidth = 20
	cw.StructuredConfig = &cfg

	Printw("message", AInt("i", 1), AString("receiver", "pkg"))

	DefaultLogger.Printw("message", Attrs{{"i", 3}, {"receiver", "logger"}}...)

	tr := Start()
	tr.Printw("message", Attrs{{"i", 5}, {"receiver", "trace"}}...)
	tr.Finish()

	tr = Span{}
	tr.Printw("message", Attrs{{"i", 7}}...)

	Printw("msg", Attrs{{"quoted", `a=b`}}...)
	Printw("msg", Attrs{{"quoted", `q"w"e`}}...)

	Printw("msg", Attrs{{"empty", ``}}...)
	cfg.QuoteEmptyValue = true
	Printw("msg", Attrs{{"empty", ``}}...)

	Printw("msg", Attrs{{"difflen", `a`}, {"next", "val"}}...)
	Printw("msg", Attrs{{"difflen", `abcde`}, {"next", "val"}}...)
	Printw("msg", Attrs{{"difflen", `ab`}, {"next", "val"}}...)

	assert.Equal(t, `message             i=1  receiver=pkg
message             i=3  receiver=logger
message             i=5  receiver=trace
msg                 quoted="a=b"
msg                 quoted="q\"w\"e"
msg                 empty=
msg                 empty=""
msg                 difflen=a  next=val
msg                 difflen=abcde  next=val
msg                 difflen=ab     next=val
`, buf.String())
}

func TestLabelRegexp(t *testing.T) {
	l := New()

	l.SetLabels(Labels{"a", "b=c"})

	assert.Panics(t, func() {
		l.SetLabels(Labels{"!a", "b=c"})
	})
}

func TestMetrics(t *testing.T) {
	tm := time.Date(2019, time.July, 6, 19, 45, 25, 0, time.Local)
	now = func() time.Time {
		tm = tm.Add(time.Second)
		return tm
	}

	var w collectWriter

	DefaultLogger = New(&w)
	DefaultLogger.NoCaller = true

	RegisterMetric("name1", MGauge, "help 1", Labels{"label1"})
	Observe("name1", 4, Labels{"label11"})

	DefaultLogger.RegisterMetric("name2", MCounter, "help 2", Labels{"label2"})
	DefaultLogger.Observe("name2", 2, Labels{"label22"})

	tr := Start()

	tr.Observe("name2", 5, Labels{"label33"})

	tr.Finish()

	assert.Equal(t, []cev{
		{Ev: Meta{Type: MetaMetricDescription, Data: Labels{"name=name1", "type=" + MGauge, "help=help 1", "labels", "label1"}}},
		{Ev: Metric{Name: "name1", Value: 4, Labels: Labels{"label11"}}},
		{Ev: Meta{Type: MetaMetricDescription, Data: Labels{"name=name2", "type=" + MCounter, "help=help 2", "labels", "label2"}}},
		{Ev: Metric{Name: "name2", Value: 2, Labels: Labels{"label22"}}},
		{Ev: SpanStart{ID: tr.ID, StartedAt: tr.StartedAt.UnixNano()}},
		{ID: tr.ID, Ev: Metric{Name: "name2", Value: 5, Labels: Labels{"label33"}}},
		{Ev: SpanFinish{ID: tr.ID, Elapsed: time.Second.Nanoseconds()}},
	}, w.Events)

	// regexp
	assert.Panics(t, func() {
		DefaultLogger.RegisterMetric("qwe123!", "", "", nil)
	})

	assert.Panics(t, func() {
		DefaultLogger.RegisterMetric("qwe123", "", "", Labels{"a=!"})
	})
}

//nolint:wsl
func TestVerbosity(t *testing.T) {
	tm := time.Date(2019, time.July, 5, 23, 49, 40, 0, time.Local)
	now = func() time.Time {
		tm = tm.Add(time.Second)
		return tm
	}

	assert.Equal(t, "", (*Logger)(nil).Filter())

	var buf bytes.Buffer

	DefaultLogger = New(NewConsoleWriter(&buf, Lnone))

	assert.False(t, If("any_topic"))
	V("any_topic").Printf("All conditionals are disabled by default")

	SetFilter("topic1,tlog=topic3")

	assert.Equal(t, "topic1,tlog=topic3", Filter())

	Printf("unconditional message")

	assert.True(t, DefaultLogger.If("topic1"))
	assert.False(t, DefaultLogger.If("topic2"))

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
	if tr.If("TRACE") {
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

	DefaultLogger = nil
	V("a").Printf("none")
}

func TestSetFilter(t *testing.T) {
	const N = 100

	l := New(Discard)

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

	tr2 = SpawnOrStart(ID{})
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

	res, err = IDFromStringAsBytes([]byte(id.FullString()))
	assert.NoError(t, err)
	assert.Equal(t, id, res)

	res, err = IDFromBytes(id[:])
	assert.NoError(t, err)
	assert.Equal(t, id, res)

	res, err = IDFromString(fmt.Sprintf("%8x", id))
	assert.Equal(t, TooShortIDError{N: 4}, err)
	assert.Equal(t, ID{1, 2, 3, 4}, res)

	res, err = IDFromStringAsBytes([]byte(fmt.Sprintf("%8x", id)))
	assert.Equal(t, TooShortIDError{N: 4}, err)
	assert.Equal(t, ID{1, 2, 3, 4}, res)

	res, err = IDFromBytes(id[:4])
	assert.Equal(t, TooShortIDError{N: 4}, err)
	assert.Equal(t, ID{1, 2, 3, 4}, res)

	res, err = IDFromString("010203046q")
	assert.EqualError(t, err, "encoding/hex: invalid byte: U+0071 'q'")
	assert.Equal(t, ID{1, 2, 3, 4, 0x60}, res)

	res, err = IDFromString(ID{}.FullString())
	assert.NoError(t, err)
	assert.Equal(t, ID{}, res)
}

func TestIDFromMustShould(t *testing.T) {
	id := ID{1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 0xa, 0xb}

	// strings
	// should
	res := ShouldID(IDFromString(id.FullString()))
	assert.Equal(t, id, res)

	res = ShouldID(IDFromString(id.String()))
	assert.Equal(t, ID{1, 2, 3, 4, 5, 6, 7, 8}, res)

	res = ShouldID(IDFromString(ID{}.FullString()))
	assert.Equal(t, ID{}, res)

	res = ShouldID(IDFromString(ID{}.String()))
	assert.Equal(t, ID{}, res)

	// must
	res = MustID(IDFromString(id.FullString()))
	assert.Equal(t, id, res)

	assert.Panics(t, func() { MustID(IDFromString(id.String())) })

	assert.Panics(t, func() { MustID(IDFromString("1234567")) })

	b := make([]byte, len(id)*2)
	id.FormatTo(b, 'x')
	b[10] = 'g'

	assert.Panics(t, func() { MustID(IDFromString(string(b))) })

	res = MustID(IDFromString(ID{}.FullString()))
	assert.Equal(t, ID{}, res)

	assert.NotPanics(t, func() { MustID(IDFromString(ID{}.String())) })

	// bytes
	res = ShouldID(IDFromBytes(nil))
	assert.Equal(t, ID{}, res)

	res = ShouldID(IDFromBytes(id[:]))
	assert.Equal(t, id, res)

	assert.Panics(t, func() { MustID(IDFromBytes([]byte{1, 2, 3, 4, 5})) })

	res = MustID(IDFromBytes(id[:]))
	assert.Equal(t, id, res)
}

func TestJSONWriterSpans(t *testing.T) {
	tm := time.Date(2019, time.July, 7, 16, 31, 10, 0, time.UTC)
	now = func() time.Time {
		tm = tm.Add(time.Second)
		return tm
	}

	var buf bytes.Buffer
	w := NewJSONWriter(&buf)
	l := New(w)
	l.randID = testRandID()

	l.SetLabels(Labels{"a=b", "f"})

	l.RegisterMetric("metric_name", "type", "help description", Labels{"const=labels"})

	tr := l.Start()

	tr.SetLabels(Labels{"a=d", "g"})

	tr1 := l.Spawn(tr.ID)

	tr1.Printf("message %d", 2)

	tr1.PrintRaw(0, LevelError, "link to %v", Args{"ID"}, Attrs{{"id", tr.ID}, {"str", "str_value"}})

	tr1.Observe("metric_name", 123.456789, Labels{"q=w", "e=1"})
	tr1.Observe("metric_name", 456.123, Labels{"q=w", "e=1"})

	tr1.Finish()

	tr.Finish()

	//nolint:lll
	re := `{"L":{"L":\["a=b","f"\]}}
{"M":{"t":"metric_desc","d":\["name=metric_name","type=type","help=help description","labels","const=labels"\]}}
{"l":{"p":\d+,"e":\d+,"f":"[\w./-]*tlog_test.go","l":\d+,"n":"github.com/nikandfor/tlog.TestJSONWriterSpans"}}
{"s":{"i":"0194fdc2fa2ffcc041d3ff12045b73c8","s":1562517071000000000,"l":\d+}}
{"L":{"s":"0194fdc2fa2ffcc041d3ff12045b73c8","L":\["a=d","g"\]}}
{"s":{"i":"6e4ff95ff662a5eee82abdf44a2d0b75","s":1562517072000000000,"l":\d+,"p":"0194fdc2fa2ffcc041d3ff12045b73c8"}}
{"l":{"p":\d+,"e":\d+,"f":"[\w./-]*tlog_test.go","l":\d+,"n":"github.com/nikandfor/tlog.TestJSONWriterSpans"}}
{"m":{"s":"6e4ff95ff662a5eee82abdf44a2d0b75","t":1562517073000000000,"l":\d+,"m":"message 2"}}
{"l":{"p":\d+,"e":\d+,"f":"[\w./-]*tlog_test.go","l":\d+,"n":"github.com/nikandfor/tlog.TestJSONWriterSpans"}}
{"m":{"s":"6e4ff95ff662a5eee82abdf44a2d0b75","t":1562517074000000000,"l":\d+,"m":"link to ID","i":"E","a":\[{"n":"id","t":"d","v":"0194fdc2fa2ffcc041d3ff12045b73c8"},{"n":"str","t":"s","v":"str_value"}\]}}
{"v":{"s":"6e4ff95ff662a5eee82abdf44a2d0b75","h":\d+,"v":123.456789,"n":"metric_name","L":\["q=w","e=1"\]}}
{"v":{"s":"6e4ff95ff662a5eee82abdf44a2d0b75","h":\d+,"v":456.123}}
{"f":{"i":"6e4ff95ff662a5eee82abdf44a2d0b75","e":3000000000}}
{"f":{"i":"0194fdc2fa2ffcc041d3ff12045b73c8","e":5000000000}}
`

	bl := strings.Split(buf.String(), "\n")

	for i, rel := range strings.Split(re, "\n") {
		ok, err := regexp.Match(rel, []byte(bl[i]))
		assert.NoError(t, err)
		assert.True(t, ok, "expected:\n%v\nactual:\n%v\n", rel, bl[i])
	}
}

func TestAppendWriter(t *testing.T) {
	l := New()

	assert.Equal(t, l.Writer, Discard)

	l.AppendWriter()

	assert.Equal(t, l.Writer, Discard)

	l.AppendWriter(Discard)

	assert.Equal(t, l.Writer, Discard)

	l.AppendWriter(Discard, Discard)

	assert.Equal(t, l.Writer, TeeWriter{Discard, Discard})

	l.AppendWriter(Discard, Discard)

	assert.Equal(t, l.Writer, TeeWriter{Discard, Discard, Discard, Discard})

	jw := NewJSONWriter(nil)
	l = New(jw)

	l.AppendWriter(Discard)

	assert.Equal(t, l.Writer, TeeWriter{jw, Discard})
}

func TestRandID(t *testing.T) {
	l := New()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		l.Printf("msg")

		wg.Done()
	}()

	id := l.RandID()
	assert.NotZero(t, id)

	id = (*Logger)(nil).RandID()
	assert.Zero(t, id)

	wg.Wait()
}

func TestCoverUncovered(t *testing.T) {
	var buf bytes.Buffer
	DefaultLogger = New(NewJSONWriter(&buf))

	assert.True(t, DefaultLogger.Valid())
	assert.False(t, (*Logger)(nil).Valid())

	SetLabels(Labels{"a", "q"})

	assert.Equal(t, `{"L":{"L":["a","q"]}}`+"\n", buf.String())

	(*Logger)(nil).SetLabels(Labels{"a"})

	(*Logger)(nil).SetFilter("any")

	assert.Equal(t, "too short id: 7, wanted 16", TooShortIDError{N: 7}.Error())

	assert.Equal(t, "1", fmt.Sprintf("%01v", ID{0x12, 0x34, 0x56}))

	b := make([]byte, 8)
	ID{0xaa, 0xbb, 0xcc, 0x44, 0x55}.FormatTo(b, 'X')
	assert.Equal(t, "AABBCC44", string(b))

	ID{}.FormatTo(b, 'x')
	assert.Equal(t, "00000000", string(b))

	id := DefaultLogger.stdRandID()
	assert.NotZero(t, id)

	tr := NewSpan(nil, ID{1, 2, 3}, 0)
	assert.Zero(t, tr)

	tr = NewSpan(DefaultLogger, ID{1, 2, 3}, 0)
	assert.True(t, DefaultLogger == tr.Logger)
	assert.NotZero(t, tr.ID)

	var w collectWriter
	l := New(&w)
	l.NoCaller = true
	l.randID = func() ID { return ID{4, 5, 6} }
	now = func() time.Time {
		return time.Unix(0, 0)
	}

	(*Logger)(nil).SpawnOrStart(ID{1, 2, 3})

	tr = l.SpawnOrStart(ID{1, 2, 3})
	assert.True(t, l == tr.Logger)

	tr = l.SpawnOrStart(ID{})
	assert.True(t, l == tr.Logger)

	assert.Equal(t, []cev{
		{Ev: SpanStart{ID: ID{4, 5, 6}, Parent: ID{1, 2, 3}}},
		{Ev: SpanStart{ID: ID{4, 5, 6}}},
	}, w.Events)

	w.Events = w.Events[:0]

	tr = l.Migrate(Span{ID: ID{1, 2, 3}})
	assert.Zero(t, tr)

	tr = l.Migrate(Span{Logger: DefaultLogger, ID: ID{4, 5, 6}})
	assert.True(t, l == tr.Logger)
	assert.Equal(t, []cev{}, w.Events)

	w.Events = w.Events[:0]

	l.randID = func() ID { return ID{7, 8, 9} }

	tr = tr.Spawn()
	_ = Span{}.Spawn()
	assert.Equal(t, []cev{
		{Ev: SpanStart{ID: ID{7, 8, 9}, Parent: ID{4, 5, 6}}},
	}, w.Events)

	var cw CountableIODiscard

	n, err := cw.Write([]byte("qwert"))
	assert.NoError(t, err)
	assert.Equal(t, 5, n)
}

func TestPrintfVsPrintln(t *testing.T) {
	var buf bytes.Buffer

	l := New(NewConsoleWriter(&buf, 0))

	l.Printf("message %v %v %v", 1, 2, 3)

	a := buf.String()
	buf.Reset()

	l.Println("message", 1, 2, 3)

	b := buf.String()

	assert.Equal(t, a, b)
}

func BenchmarkPrintfVsPrintln(b *testing.B) {
	b.ReportAllocs()

	l := New(NewConsoleWriter(ioutil.Discard, 0))
	l.NoCaller = true

	for i := 0; i < b.N; i++ {
		l.Printf("message %d %d %d", i, i+1, i+2)
		l.Println("message", i, i+1, i+2)
	}
}

func BenchmarkRand(b *testing.B) {
	b.Run("Std", func(b *testing.B) {
		b.RunParallel(func(b *testing.PB) {
			var id ID
			for b.Next() {
				_, _ = rand.Read(id[:])
			}
		})
	})

	b.Run("Crypto", func(b *testing.B) {
		b.RunParallel(func(b *testing.PB) {
			var id ID
			for b.Next() {
				_, _ = crand.Read(id[:])
			}
		})
	})
}

func BenchmarkStdLogLogger(b *testing.B) {
	for _, tc := range []struct {
		name string
		ff   int
	}{
		{"Std", log.LstdFlags},
		{"Det", log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile},
	} {
		tc := tc

		b.Run(tc.name, func(b *testing.B) {
			l := log.New(ioutil.Discard, "", tc.ff)

			b.Run("SingleThread", func(b *testing.B) {
				b.ReportAllocs()

				for i := 0; i < b.N; i++ {
					l.Printf("message: %d", 1000+i)
				}
			})

			b.Run(Parallel, func(b *testing.B) {
				b.ReportAllocs()

				b.RunParallel(func(b *testing.PB) {
					i := 0
					for b.Next() {
						i++
						l.Printf("message: %d", 1000+i)
					}
				})
			})
		})
	}
}

//nolint:gocognit
func BenchmarkTlogLogger(b *testing.B) {
	for _, tc := range []struct {
		name string
		ff   int
	}{
		{"Std", LstdFlags},
		{"Det", LdetFlags},
	} {
		tc := tc

		b.Run(tc.name, func(b *testing.B) {
			l := New(NewConsoleWriter(ioutil.Discard, tc.ff))
			if tc.ff == LstdFlags {
				l.NoCaller = true
			}

			cases := []struct {
				name string
				act  func(i int)
			}{
				{"Printf", func(i int) { l.Printf("message: %d", 1000+i) }},
				{"Printw", func(i int) { l.Printw("message", AInt("i", 1000+i)) }},
			}

			for _, par := range []bool{false, true} {
				par := par

				if !par {
					b.Run("SingleThread", func(b *testing.B) {
						for _, tc := range cases {
							tc := tc

							b.Run(tc.name, func(b *testing.B) {
								b.ReportAllocs()

								for i := 0; i < b.N; i++ {
									tc.act(i)
								}
							})
						}
					})
				} else {
					b.Run(Parallel, func(b *testing.B) {
						for _, tc := range cases {
							tc := tc

							b.Run(tc.name, func(b *testing.B) {
								b.ReportAllocs()

								b.RunParallel(func(b *testing.PB) {
									i := 0
									for b.Next() {
										i++
										tc.act(i)
									}
								})
							})
						}
					})
				}
			}
		})
	}
}

//nolint:gocognit
func BenchmarkTlogTracer(b *testing.B) {
	for _, ws := range []struct {
		name string
		nw   func(w io.Writer) Writer
	}{
		{"ConsoleStd", func(w io.Writer) Writer {
			return NewConsoleWriter(w, LstdFlags)
		}},
		/*
			{"ConsoleDet", func(w io.Writer) Writer {
				return NewConsoleWriter(w, LdetFlags)
			}},
		*/
		{"JSON", func(w io.Writer) Writer {
			return NewJSONWriter(w)
		}},
		{"Proto", func(w io.Writer) Writer {
			return NewProtoWriter(w)
		}},
		{"Discard", func(w io.Writer) Writer {
			return Discard
		}},
	} {
		ws := ws

		b.Run(ws.name, func(b *testing.B) {
			for _, par := range []bool{false, true} {
				par := par

				n := SingleThread
				if par {
					n = Parallel
				}

				b.Run(n, func(b *testing.B) {
					buf := []byte("raw message") // reusable buffer
					ls := Labels{"const=label", "couple=of_them"}

					var w CountableIODiscard
					l := New(ws.nw(&w))

					gtr := l.Start()

					for _, tc := range []struct {
						name string
						act  func(i int)
					}{
						{"StartFinish", func(i int) {
							tr := l.Start()
							tr.Finish()
						}},
						{"Printf", func(i int) {
							gtr.Printf("message: %d", 1000+i) // 1 alloc here: int to interface{} conversion
						}},
						{"Printw", func(i int) {
							gtr.Printw("message", AInt("i", 1000+i)) // 1 alloc here: int to interface{} conversion
						}},
						{"PrintBytes", func(i int) {
							gtr.PrintBytes(0, buf)
						}},
						{"StartPrintfFinish", func(i int) {
							tr := l.Start()
							tr.Printf("message: %d", 1000+i) // 1 alloc here: int to interface{} conversion
							tr.Finish()
						}},
						{"Metric", func(i int) {
							gtr.Observe("metric_full_qualified_name_unit", 123.456, ls)
						}},
					} {
						tc := tc

						b.Run(tc.name, func(b *testing.B) {
							b.ReportAllocs()
							w.N, w.B = 0, 0

							if par {
								b.RunParallel(func(b *testing.PB) {
									i := 0
									for b.Next() {
										i++
										tc.act(i)
									}
								})
							} else {
								for i := 0; i < b.N; i++ {
									tc.act(i)
								}
							}

							w.ReportDisk(b)
						})
					}
				})
			}
		})
	}
}

func BenchmarkTlogProtoWrite(b *testing.B) {
	b.ReportAllocs()

	l := New(NewProtoWriter(ioutil.Discard))

	tr := l.Start()
	w := tr.IOWriter(0)

	buf := AppendPrintf(nil, "message %d", 1000)

	for i := 0; i < b.N; i++ {
		_, _ = w.Write(buf)
	}

	tr.Finish()
}

func BenchmarkIDFormat(b *testing.B) {
	b.ReportAllocs()

	id := ID{1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 0xa, 0xb, 0xc, 0xd, 0xe, 0xf}

	for i := 0; i < b.N; i++ {
		fmt.Fprintf(ioutil.Discard, "%+x", id)
	}
}

func BenchmarkIDFormatTo(b *testing.B) {
	b.ReportAllocs()

	var buf [40]byte
	id := ID{1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 0xa, 0xb, 0xc, 0xd, 0xe, 0xf}

	for i := 0; i < b.N; i++ {
		if i&0xf == 0 {
			ID{}.FormatTo(buf[:], 'v')
		} else {
			id.FormatTo(buf[:], 'v')
		}
	}
}

//nolint:gocognit
func TestTlogGrandParallel(t *testing.T) {
	const N = 10000

	now = time.Now

	var buf0, buf1, buf2 bytes.Buffer

	DefaultLogger = New(NewConsoleWriter(LockWriter(&buf0), LdetFlags), NewJSONWriter(LockWriter(&buf1)), NewProtoWriter(LockWriter(&buf2)))

	var wg sync.WaitGroup

	wg.Add(14)

	tr := Start()

	for j := 0; j < 2; j++ {
		go func() {
			defer wg.Done()

			for i := 0; i < N; i++ {
				switch i & 2 {
				case 0:
					SetFilter("")
				case 1:
					SetFilter("a")
				}
			}
		}()
	}

	for j := 0; j < 2; j++ {
		go func() {
			defer wg.Done()

			for i := 0; i < N; i++ {
				DefaultLogger.Printf("message %d", i)
			}
		}()
	}

	for j := 0; j < 2; j++ {
		go func() {
			defer wg.Done()

			for i := 0; i < N; i++ {
				V("a").Printf("message %d", i)
			}
		}()
	}

	for j := 0; j < 2; j++ {
		go func() {
			defer wg.Done()

			for i := 0; i < N; i++ {
				tr := DefaultLogger.Start()
				tr.Printf("message %d", i)
				tr.Finish()
			}
		}()
	}

	for j := 0; j < 2; j++ {
		go func() {
			defer wg.Done()

			for i := 0; i < N; i++ {
				tr := DefaultLogger.V("b").Start()
				tr.Printf("message %d", i)
				tr.Finish()
			}
		}()
	}

	for j := 0; j < 2; j++ {
		go func() {
			defer wg.Done()

			for i := 0; i < N; i++ {
				tr := Start()
				tr.V("a").Printf("message %d", i)
				tr.Finish()
			}
		}()
	}

	for j := 0; j < 2; j++ {
		go func() {
			defer wg.Done()

			for i := 0; i < N; i++ {
				tr.Printf("message %d", i)
			}
		}()
	}

	wg.Wait()
}

func testNow(tm *time.Time) func() time.Time {
	var mu sync.Mutex

	return func() time.Time {
		defer mu.Unlock()
		mu.Lock()

		*tm = tm.Add(time.Second)

		return *tm
	}
}
