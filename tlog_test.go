package tlog

import (
	crand "crypto/rand"
	"encoding/hex"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"sync"
	"testing"
	"time"
)

const SingleThread, Parallel = "SingleThread", "Parallel"

func TestLoggerSmoke(t *testing.T) {
	var b bufWriter
	l := New(&b)

	l.Printf("message: %v %v", 1, "two")

	t.Logf("data:\n%v", hex.Dump(b))
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

	/*
		b.Run("fast", func(b *testing.B) {
			b.RunParallel(func(b *testing.PB) {
				var id ID
				for b.Next() {
					id = FastRandID()
				}
				_ = id
			})
		})
	*/
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

//nolint:gocognit,dupl
func BenchmarkTlogLogger(b *testing.B) {
	for _, tc := range []struct {
		name string
		new  func(w io.Writer) *Logger
	}{
		{"NCallerYTime", func(w io.Writer) *Logger { l := New(w); l.NoCaller = true; return l }},
		{"NCallerNTime", func(w io.Writer) *Logger { l := New(w); l.NoCaller = true; l.NoTime = true; return l }},
		{"YCallerYTime", func(w io.Writer) *Logger { return New(w) }},
	} {
		tc := tc

		b.Run(tc.name, func(b *testing.B) {
			l := tc.new(ioutil.Discard)

			cases := []struct {
				name string
				act  func(i int)
			}{
				{"PrintBytes", func(i int) { l.Printf("message: 1000") }},
				{"Printf", func(i int) { l.Printf("message: %d", 1000+i) }},
				{"Printw", func(i int) {
					l.Printw("message",
						"str", "string",
						"i", 1000+i,
					)
				}},
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

func testNow(tm *time.Time) func() time.Time {
	var mu sync.Mutex

	return func() time.Time {
		defer mu.Unlock()
		mu.Lock()

		*tm = tm.Add(time.Second)

		return *tm
	}
}
