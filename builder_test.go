package tlog

import (
	"io"
	"io/ioutil"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMessageBuilder(t *testing.T) {
	tm := time.Date(2019, time.July, 7, 16, 31, 10, 0, time.UTC)
	now = func() time.Time {
		tm = tm.Add(time.Second)
		return tm
	}

	var b bufWriter

	l := New(NewJSONWriter(&b))
	l.NewID = testRandID()

	l.BuildMessage().Caller(0).Now().Int("int", 12).Printf("message: %v", "args")

	var loc PC

	tr := l.BuildSpanStart().NewID().CallerOnce(0, &loc).Now().Start()

	tr.BuildMessage().Now().Str("str", "str_value").Printf("")

	tr.Finish()

	re := `{"l":{"p":\d+,"e":\d+,"f":"[\w./-]*builder_test.go","l":\d+,"n":"github.com/nikandfor/tlog.TestMessageBuilder"}}
{"m":{"t":1562517071000000000,"l":\d+,"m":"message: args","a":\[{"n":"int","t":"i","v":12}\]}}
{"l":{"p":\d+,"e":\d+,"f":"[\w./-]*builder_test.go","l":\d+,"n":"github.com/nikandfor/tlog.TestMessageBuilder"}}
{"s":{"i":"0194fdc2fa2ffcc041d3ff12045b73c8","s":1562517072000000000,"l":\d+}}
{"m":{"s":"0194fdc2fa2ffcc041d3ff12045b73c8","t":1562517073000000000,"a":\[{"n":"str","t":"s","v":"str_value"}\]}}
{"f":{"i":"0194fdc2fa2ffcc041d3ff12045b73c8","e":2000000000}}
`

	bl := strings.Split(string(b), "\n")

	for i, rel := range strings.Split(re, "\n") {
		ok, err := regexp.Match(rel, []byte(bl[i]))
		assert.NoError(t, err)
		assert.True(t, ok, "expected:\n%v\nactual:\n%v\n", rel, bl[i])
	}
}

//nolint:gocognit,dupl
func BenchmarkBuilder(b *testing.B) {
	for _, tc := range []struct {
		name string
		new  func(w io.Writer) *Logger
	}{
		{"Std", func(w io.Writer) *Logger { l := New(NewConsoleWriter(w, LstdFlags)); l.NoCaller = true; return l }},
		{"Det", func(w io.Writer) *Logger { return New(NewConsoleWriter(w, LdetFlags)) }},
		{"JSON", func(w io.Writer) *Logger { return New(NewJSONWriter(w)) }},
		{"Proto", func(w io.Writer) *Logger { return New(NewProtoWriter(w)) }},
		{"Discard", func(w io.Writer) *Logger { return New() }},
	} {
		tc := tc

		b.Run(tc.name, func(b *testing.B) {
			l := tc.new(ioutil.Discard)
			var loc PC

			cases := []struct {
				name string
				act  func(i int)
			}{
				{"Message", func(i int) { l.BuildMessage().Now().Caller(0).Printf("message: %d", 1000+i) }},
				{"MessageLocOnce", func(i int) { l.BuildMessage().Now().CallerOnce(0, &loc).Printf("message: %d", 1000+i) }},
				{"MessageAttrs", func(i int) { l.BuildMessage().Now().Caller(0).Int("i", 1000+i).Printf("message") }},
				{"MessageAttrsLocOnce", func(i int) { l.BuildMessage().Now().CallerOnce(0, &loc).Int("i", 1000+i).Printf("message") }},
				{"SpanStartFinish", func(i int) { l.BuildSpanStart().NewID().Now().Start().Finish() }},
			}

			for _, par := range []bool{false, true} {
				par := par

				if !par {
					b.Run(SingleThread, func(b *testing.B) {
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

func BenchmarkBuilderCallerOnce(b *testing.B) {
	b.Run("Caller", func(b *testing.B) {
		m := DefaultLogger.BuildMessage()

		b.RunParallel(func(b *testing.PB) {
			for b.Next() {
				m.Caller(2)
			}
		})
	})

	b.Run("CallerOnce", func(b *testing.B) {
		var loc PC
		m := DefaultLogger.BuildMessage()

		b.RunParallel(func(b *testing.PB) {
			for b.Next() {
				m.CallerOnce(2, &loc)
			}
		})
	})
}

//nolint:dupl
func BenchmarkMutex(b *testing.B) {
	b.Run("Mutex", func(b *testing.B) {
		var mu sync.Mutex

		b.Run("SingleThread", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				mu.Lock()
				mu.Unlock() //nolint:staticcheck
			}
		})

		b.Run("Parallel", func(b *testing.B) {
			b.RunParallel(func(b *testing.PB) {
				for b.Next() {
					mu.Lock()
					mu.Unlock() //nolint:staticcheck
				}
			})
		})
	})

	b.Run("RWMutex", func(b *testing.B) {
		var mu sync.RWMutex

		b.Run("SingleThread", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				mu.RLock()
				mu.RUnlock() //nolint:staticcheck
			}
		})

		b.Run("Parallel", func(b *testing.B) {
			b.RunParallel(func(b *testing.PB) {
				for b.Next() {
					mu.RLock()
					mu.RUnlock() //nolint:staticcheck
				}
			})
		})
	})
}
