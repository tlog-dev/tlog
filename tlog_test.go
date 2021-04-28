package tlog

import (
	"io/ioutil"
	"sync"
	"testing"

	"github.com/nikandfor/tlog/low"
)

func TestLoggerSmoke(t *testing.T) {
	var buf low.Buf

	l := New(&buf)

	l.Printf("message %v %v", 1, "two")

	t.Logf("dump:\n%v", Dump(buf))
	buf = buf[:0]

	l.Printw("message", "a", -1, "b", "two")

	t.Logf("dump:\n%v", Dump(buf))
	buf = buf[:0]

	l.NewMessage(0, ID{}, "")

	t.Logf("dump:\n%v", Dump(buf))
	buf = buf[:0]
}

func TestLoggerSmokeConcurrent(t *testing.T) {
	const N = 1000

	var wg sync.WaitGroup
	var buf low.Buf

	l := New(&buf)

	wg.Add(1)
	go func() {
		defer wg.Done()

		for i := 0; i < N; i++ {
			l.Printf("printf %v %v", i+1000, i+1001)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		for i := 0; i < N; i++ {
			l.Printw("printw", "i0", i+1000, "i1", i+1001)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		for i := 0; i < N; i++ {
			tr := l.Start("span")
			tr.Printw("span.printw", "i0", i+1000, "i1", i+1001)
			tr.Finish()
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		for i := 0; i < N; i++ {
			tr := l.Start("span_observer")
			tr.Observe("value", i+1000)
			tr.Finish()
		}
	}()

	wg.Wait()
}

func BenchmarkLoggerPrintw(b *testing.B) {
	b.ReportAllocs()

	l := New(ioutil.Discard)
	l.NoCaller = true
	l.NoTime = true

	for i := 0; i < b.N; i++ {
		l.Printw("message", "a", i+1000, "b", i+1000)
	}
}

func BenchmarkLoggerPrintf(b *testing.B) {
	b.ReportAllocs()

	l := New(ioutil.Discard)
	l.NoCaller = true
	l.NoTime = true

	for i := 0; i < b.N; i++ {
		l.Printf("message a %v b %v", i+1000, i+1000)
	}
}

func BenchmarkLoggerPrint(b *testing.B) {
	b.ReportAllocs()

	l := New(ioutil.Discard)
	l.NoCaller = true
	l.NoTime = true

	for i := 0; i < b.N; i++ {
		l.Printf("message")
	}
}

func BenchmarkTracerStartPrintwFinish(b *testing.B) {
	b.ReportAllocs()

	l := New(ioutil.Discard)
	l.NoCaller = true
	l.NoTime = true

	for i := 0; i < b.N; i++ {
		tr := l.Start("span_name")
		tr.Printw("message", "a", i+1000, "b", i+1000)
		tr.Finish()
	}
}
