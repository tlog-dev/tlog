package benchmarks

import (
	"testing"

	"github.com/golang/glog"
)

func BenchmarkGlogLogger(b *testing.B) {
	//	b.Skip("it creates files")

	b.Run("SingleThread", func(b *testing.B) {
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			glog.Infof("message: %d", 1000+i)
		}
	})

	b.Run("Parallel", func(b *testing.B) {
		b.ReportAllocs()

		b.RunParallel(func(b *testing.PB) {
			i := 0
			for b.Next() {
				i++
				glog.Infof("message: %d", 1000+i)
			}
		})
	})
}
