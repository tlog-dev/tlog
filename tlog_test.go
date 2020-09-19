//nolint:gosec
package tlog

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type testt struct{}

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

func (t *testt) testloc2() Location {
	return func() Location {
		return Caller(0)
	}()
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
	defer func(l *Logger) {
		DefaultLogger = l
	}(DefaultLogger)
	tm := time.Date(2019, time.July, 6, 19, 45, 25, 0, time.Local)

	now = func() int64 {
		tm = tm.Add(time.Second)
		return tm.UnixNano()
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

	n, err = (*Logger)(nil).Write([]byte("123"))
	assert.NoError(t, err)
	assert.Equal(t, 3, n)
}

func TestPrintw(t *testing.T) {
	var buf bytes.Buffer
	DefaultLogger = New(NewConsoleWriter(&buf, 0))
	DefaultLogger.randID = testRandID()

	cfg := DefaultStructuredConfig
	cfg.MessageWidth = 20
	DefaultLogger.StructuredConfig = &cfg

	Printw("message", "i", 1, "receiver", "pkg")
	PrintwDepth(0, "message", "i", 2, "receiver", "pkg")

	DefaultLogger.Printw("message", "i", 3, "receiver", "logger")
	DefaultLogger.PrintwDepth(0, "message", "i", 4, "receiver", "logger")

	tr := Start()
	tr.Printw("message", "i", 5, "receiver", "trace")
	tr.PrintwDepth(0, "message", "i", 6, "receiver", "trace")
	tr.Finish()

	tr = Span{}
	tr.Printw("message", "i", 7)

	Printw("msg", "quoted", `a=b`)
	Printw("msg", "quoted", `q"w"e`)

	Printw("msg", "empty", ``)
	cfg.QuoteEmptyValue = true
	Printw("msg", "empty", ``)

	Printw("msg", "difflen", `a`, "next", "val")
	Printw("msg", "difflen", `abcde`, "next", "val")
	Printw("msg", "difflen", `ab`, "next", "val")

	assert.Equal(t, `message             i=1  receiver=pkg
message             i=2  receiver=pkg
message             i=3  receiver=logger
message             i=4  receiver=logger
message             span=0194fdc2  i=5  receiver=trace
message             span=0194fdc2  i=6  receiver=trace
msg                 quoted="a=b"
msg                 quoted="q\"w\"e"
msg                 empty=
msg                 empty=""
msg                 difflen=a  next=val
msg                 difflen=abcde  next=val
msg                 difflen=ab     next=val
`, buf.String())
}

//nolint:wsl
func TestVerbosity(t *testing.T) {
	defer func(old func() int64) {
		now = old
	}(now)
	tm := time.Date(2019, time.July, 5, 23, 49, 40, 0, time.Local)
	now = func() int64 {
		tm = tm.Add(time.Second)
		return tm.UnixNano()
	}

	assert.Equal(t, "", (*Logger)(nil).Filter())

	var buf bytes.Buffer

	DefaultLogger = New(NewConsoleWriter(&buf, Lnone))

	V("any_topic").Printf("All conditionals are disabled by default")

	SetFilter("topic1,tlog=topic3")

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
	defer func(f func() int64) {
		now = f
	}(now)
	tm := time.Date(2019, time.July, 7, 16, 31, 10, 0, time.UTC)
	now = func() int64 {
		tm = tm.Add(time.Second)
		return tm.UnixNano()
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

	tr1.Observe("metric_name", 123.456789, Labels{"q=w", "e=1"})
	tr1.Observe("metric_name", 456.123, Labels{"q=w", "e=1"})

	tr1.Finish()

	tr.Finish()

	re := `{"L":{"L":\["a=b","f"\]}}
{"M":{"t":"metric_desc","d":\["name=metric_name","type=type","help=help description","labels","const=labels"\]}}
{"l":{"p":\d+,"e":\d+,"f":"[\w./-]*tlog_test.go","l":\d+,"n":"github.com/nikandfor/tlog.TestJSONWriterSpans"}}
{"s":{"i":"0194fdc2fa2ffcc041d3ff12045b73c8","s":1562517071000000000,"l":\d+}}
{"L":{"s":"0194fdc2fa2ffcc041d3ff12045b73c8","L":\["a=d","g"\]}}
{"s":{"i":"6e4ff95ff662a5eee82abdf44a2d0b75","s":1562517072000000000,"l":\d+,"p":"0194fdc2fa2ffcc041d3ff12045b73c8"}}
{"l":{"p":\d+,"e":\d+,"f":"[\w./-]*tlog_test.go","l":\d+,"n":"github.com/nikandfor/tlog.TestJSONWriterSpans"}}
{"m":{"s":"6e4ff95ff662a5eee82abdf44a2d0b75","t":1562517073000000000,"l":\d+,"m":"message 2"}}
{"v":{"s":"6e4ff95ff662a5eee82abdf44a2d0b75","h":\d+,"v":123.456789,"n":"metric_name","L":\["q=w","e=1"\]}}
{"v":{"s":"6e4ff95ff662a5eee82abdf44a2d0b75","h":\d+,"v":456.123}}
{"f":{"i":"6e4ff95ff662a5eee82abdf44a2d0b75","e":2000000000}}
{"f":{"i":"0194fdc2fa2ffcc041d3ff12045b73c8","e":4000000000}}
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

	l.AppendWriter(Discard)

	assert.Equal(t, l.Writer, Discard)

	l.AppendWriter(Discard, Discard)

	assert.Equal(t, l.Writer, TeeWriter{Discard, Discard})

	l.AppendWriter(Discard, Discard)

	assert.Equal(t, l.Writer, TeeWriter{Discard, Discard, Discard, Discard})
}

func TestCoverUncovered(t *testing.T) {
	defer func(l *Logger) {
		DefaultLogger = l
	}(DefaultLogger)

	var buf bytes.Buffer
	DefaultLogger = New(NewJSONWriter(&buf))

	SetLabels(Labels{"a", "q"})

	assert.Equal(t, `{"L":{"L":["a","q"]}}`+"\n", buf.String())

	(*Logger)(nil).SetLabels(Labels{"a"})

	assert.Equal(t, "too short id: 7, wanted 32", TooShortIDError{N: 7}.Error())

	assert.Equal(t, "1", fmt.Sprintf("%01v", ID{0x12, 0x34, 0x56}))

	b := make([]byte, 8)
	ID{0xaa, 0xbb, 0xcc, 0x44, 0x55}.FormatTo(b, 'X')
	assert.Equal(t, "AABBCC44", string(b))

	ID{}.FormatTo(b, 'x')
	assert.Equal(t, "00000000", string(b))

	id := DefaultLogger.stdRandID()
	assert.NotZero(t, id)
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
	l.NoLocations = true

	for i := 0; i < b.N; i++ {
		l.Printf("message %d %d %d", i, i+1, i+2)
		l.Println("message", i, i+1, i+2)
	}
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

			b.Run("Parallel", func(b *testing.B) {
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
				l.NoLocations = true
			}

			cases := []struct {
				name string
				act  func(i int)
			}{
				{"Printf", func(i int) { l.Printf("message: %d", 1000+i) }},
				{"Printw", func(i int) { l.Printw("message", "i", 1000+i) }},
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
					b.Run("Parallel", func(b *testing.B) {
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
func BenchmarkTlogTraces(b *testing.B) {
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

				n := "SingleThread"
				if par {
					n = "Parallel"
				}

				b.Run(n, func(b *testing.B) {
					buf := []byte("raw message") // reusable buffer
					ls := Labels{"const=label", "couple=of_them"}

					var w CountableDiscard
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
						{"PrintRaw", func(i int) {
							gtr.PrintRaw(0, buf)
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

	buf := AppendPrintf(nil, "message %d", 1000)

	for i := 0; i < b.N; i++ {
		_, _ = tr.Write(buf)
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

	now = func() int64 { return time.Now().UnixNano() }
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

type CountableDiscard struct {
	B, N int64
}

func (w *CountableDiscard) ReportDisk(b *testing.B) {
	b.ReportMetric(float64(w.B)/float64(b.N), "disk_B/op")
}

func (w *CountableDiscard) Write(p []byte) (int, error) {
	atomic.AddInt64(&w.N, 1)
	atomic.AddInt64(&w.B, int64(len(p)))

	return len(p), nil
}

func testNow(tm *time.Time) func() int64 {
	var mu sync.Mutex

	return func() int64 {
		defer mu.Unlock()
		mu.Lock()

		*tm = tm.Add(time.Second)
		return tm.UnixNano()
	}
}
