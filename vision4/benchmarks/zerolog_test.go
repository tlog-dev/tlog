package benchmarks

import (
	"testing"

	"github.com/rs/zerolog"

	"github.com/nikandfor/tlog"
)

func BenchmarkZerologLogger(b *testing.B) {
	var w tlog.CountableIODiscard

	l := zerolog.New(&w).With().Timestamp().Logger()

	b.Run("SingleThread", func(b *testing.B) {
		b.ReportAllocs()
		w.N, w.B = 0, 0

		for i := 0; i < b.N; i++ {
			l.Info().Int("i", 1000+i).Msg("message")
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
				l.Info().Int("i", 1000+i).Msg("message")
			}
		})

		//	w.ReportDisk(b)
	})
}
