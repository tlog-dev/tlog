package benchmarks

import (
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/nikandfor/tlog"
)

func BenchmarkLogrusLogger(b *testing.B) {
	var w tlog.CountableIODiscard

	l := logrus.New()
	l.Out = &w

	b.Run("SingleThread", func(b *testing.B) {
		b.ReportAllocs()
		w.N, w.B = 0, 0

		for i := 0; i < b.N; i++ {
			l.WithField("i", 1000+i).Info("message")
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
				l.WithField("i", 1000+i).Info("message")
			}
		})

		//	w.ReportDisk(b)
	})
}
