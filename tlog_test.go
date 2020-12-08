package tlog

import (
	"io/ioutil"
	"testing"

	"github.com/nikandfor/tlog/low"
)

func TestLogger(t *testing.T) {
	var buf low.Buf

	l := New(&buf)

	l.Printf("message %v %v", 1, "two")

	t.Logf("dump:\n%v", Dump(buf))
	buf = buf[:0]

	l.Printw("message", "a", 1, "b", "two")

	t.Logf("dump:\n%v", Dump(buf))
	buf = buf[:0]
}

func BenchmarkLoggerPrintf(b *testing.B) {
	b.ReportAllocs()

	l := New(ioutil.Discard)
	//	l.NoCaller = true
	//	l.NoTime = true

	for i := 0; i < b.N; i++ {
		l.Printf("message a %v b %v", i+1000, i+1000)
	}
}

func BenchmarkLoggerPrintw(b *testing.B) {
	b.ReportAllocs()

	l := New(ioutil.Discard)
	//	l.NoCaller = true
	//	l.NoTime = true

	for i := 0; i < b.N; i++ {
		l.Printw("message", "a", i+1000, "b", i+1000)
	}
}
