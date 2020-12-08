package tlog

import (
	"io"
	"io/ioutil"
	"testing"
)

const SingleThread, Parallel = "SingleThread", "Parallel"

func BenchmarkMap(b *testing.B) {
	b.ReportAllocs()

	l := New()

	for i := 0; i < b.N; i++ {
		l.Ev(nil, Info).Dict(D{"a": "b", "c": 1}).Write()
	}
}

//nolint:gocognit,dupl
func BenchmarkTlogLogger(b *testing.B) {
	for _, tc := range []struct {
		name string
		new  func(w io.Writer) *Logger
	}{
		{"Std", func(w io.Writer) *Logger {
			l := New(NewConsoleWriter(w, LstdFlags))
			l.Hooks = []Hook{AddNow}
			return l
		}},
		{"Det", func(w io.Writer) *Logger { return New(NewConsoleWriter(w, LdetFlags)) }},
		{"JSON", func(w io.Writer) *Logger { return New(NewJSONWriter(w)) }},
		{"JSON_NoLoc", func(w io.Writer) *Logger {
			l := New(NewJSONWriter(w))
			l.Hooks = []Hook{AddNow}
			return l
		}},
	} {
		tc := tc

		b.Run(tc.name, func(b *testing.B) {
			l := tc.new(ioutil.Discard)

			cases := []struct {
				name string
				act  func(i int)
			}{
				{"Printf", func(i int) { l.Printf("message: %d", 1000+i) }},
				{"Printw", func(i int) { l.Printw("message", D{"i": 1000 + i}) }},
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
