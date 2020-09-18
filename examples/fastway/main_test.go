package main

import (
	"context"
	"testing"

	"github.com/nikandfor/tlog"
)

func BenchmarkUsual(b *testing.B) {
	b.ReportAllocs()

	tlog.DefaultLogger = tlog.New(tlog.Discard)

	ctx := context.Background()
	tr := tlog.Start()
	defer tr.Finish()

	for i := 0; i < b.N; i++ {
		usualCode(tlog.ContextWithSpan(ctx, tr), 1, 2.3)
	}
}

func BenchmarkHot(b *testing.B) {
	b.ReportAllocs()

	tlog.DefaultLogger = tlog.New(tlog.Discard)

	tr := tlog.Start()
	defer tr.Finish()

	var buf []byte

	for i := 0; i < b.N; i++ {
		buf = hotCode(tr, 4, 5.6, buf)
	}
}

func BenchmarkFire(b *testing.B) {
	b.ReportAllocs()

	tlog.DefaultLogger = tlog.New(tlog.Discard)

	tr := tlog.Start()
	defer tr.Finish()

	var buf []byte

	for i := 0; i < b.N; i++ {
		buf = fire(tr, 7, 8.9, buf)
	}
}
