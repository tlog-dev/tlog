package tlog

import (
	"testing"

	"github.com/golang/glog"
)

func BenchmarkGlogLoggerDetailed(b *testing.B) {
	b.Skip() // because it creates files

	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		glog.Infof("message: %d", i)
	}
}
