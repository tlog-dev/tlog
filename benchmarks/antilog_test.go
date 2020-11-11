package benchmarks

import (
	"testing"

	"github.com/nikandfor/tlog"
	"github.com/shamaazi/antilog"
)

func BenchmarkAntilogLogger(b *testing.B) {
	var w tlog.CountableIODiscard

	l := antilog.WithWriter(&w)

	b.Run("SingleThread", func(b *testing.B) {
		b.ReportAllocs()
		w.N, w.B = 0, 0

		for i := 0; i < b.N; i++ {
			l.With("i", 1000+i).Write("message")
		}

		//	w.ReportDisk(b)
	})

	b.Run("Parallel", func(b *testing.B) {
		b.ReportAllocs()
		w.N, w.B = 0, 0

		b.RunParallel(func(b *testing.PB) {
			i := 0
			for b.Next() {
				i++
				l.With("i", 1000+i).Write("message")
			}
		})

		//	w.ReportDisk(b)
	})
}
