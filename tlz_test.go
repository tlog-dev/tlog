package tlog_test

import (
	"io"
	"testing"

	"tlog.app/go/eazy"
	"tlog.app/go/tlog"
	"tlog.app/go/tlog/tlio"
)

func BenchmarkLogCompressOneline(b *testing.B) {
	b.ReportAllocs()

	var full, small tlio.CountingIODiscard
	w := eazy.NewWriter(&small, 128*1024, 1024)

	l := tlog.New(io.MultiWriter(&full, w))
	tr := l.Start("span_name")

	types := []string{"type_a", "value_b", "qweqew", "asdads"}

	for i := range b.N {
		//	tr := l.Start("span_name")
		tr.Printw("some example message", "i", i, "type", types[i%len(types)])
		//	tr.Finish()
	}

	b.SetBytes(full.Bytes.Load() / int64(b.N))
	b.ReportMetric(float64(full.Bytes.Load())/float64(small.Bytes.Load()), "ratio")
}

func BenchmarkLogCompressOnelineText(b *testing.B) {
	b.ReportAllocs()

	var full, small tlio.CountingIODiscard
	w := eazy.NewWriter(&small, 128*1024, 1024)
	cw := tlog.NewConsoleWriter(io.MultiWriter(&full, w), tlog.LstdFlags)

	l := tlog.New(cw)
	tr := l.Start("span_name")

	types := []string{"type_a", "value_b", "qweqew", "asdads"}

	for i := range b.N {
		//	tr := l.Start("span_name")
		tr.Printw("some example message", "i", i, "type", types[i%len(types)])
		//	tr.Finish()
	}

	b.SetBytes(full.Bytes.Load() / int64(b.N))
	b.ReportMetric(float64(full.Bytes.Load())/float64(small.Bytes.Load()), "ratio")
}
