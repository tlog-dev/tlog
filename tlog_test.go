package tlog

import (
	"io/ioutil"
	"runtime"
	"sync"
	"testing"

	"github.com/nikandfor/assert"
	"github.com/nikandfor/loc"

	"github.com/nikandfor/tlog/low"
	"github.com/nikandfor/tlog/tlwire"
)

func TestLoggerSmoke(t *testing.T) {
	var buf low.Buf

	l := New(&buf)

	l.Printf("message %v %v", 1, "two")

	t.Logf("dump:\n%v", tlwire.Dump(buf))
	buf = buf[:0]

	l.Printw("message", "a", -1, "b", "two")

	t.Logf("dump:\n%v", tlwire.Dump(buf))
	buf = buf[:0]

	l.NewMessage(0, ID{}, "")

	t.Logf("dump:\n%v", tlwire.Dump(buf))
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
			_ = tr.Event("value", i+1000)
			tr.Finish()
		}
	}()

	wg.Wait()
}

func TestLoggerSetCallers(t *testing.T) {
	nextLine := func() int {
		_, _, line := loc.Caller(1).NameFileLine()
		return line + 1
	}

	var buf low.Buf
	var off int

	l := New(&buf)

	LoggerSetTimeNow(l, nil, nil)

	exp := nextLine()
	l.Printw("hello default caller")

	checkCaller(t, exp, true, buf[off:])
	off = len(buf)

	//

	LoggerSetCallers(l, 0, runtime.Callers)

	exp = nextLine()
	l.Printw("hello runtime caller")

	checkCaller(t, exp, true, buf[off:])
	off = len(buf)

	LoggerSetCallers(l, 0, func(skip int, pc []uintptr) int {
		t.Logf("skip for custom logger: %v", skip)

		pc[0] = 0x777
		loc.SetCache(loc.PC(pc[0]), "name", "file", 877)
		return 1
	})

	l.Printw("hello custom caller")

	checkCaller(t, 877, true, buf[off:])
	off = len(buf)

	LoggerSetCallers(l, 0, nil)

	l.Printw("hello no caller")

	checkCaller(t, 0, false, buf[off:])
	off = len(buf) //nolint:ineffassign,staticcheck,wastedassign

	if t.Failed() {
		t.Logf("dump:\n%v", tlwire.Dump(buf))
		buf = buf[:0]
	}
}

func checkCaller(t *testing.T, line int, exists bool, b []byte) {
	t.Helper()

	var d tlwire.Decoder
	var msg []byte
	var pc loc.PC
	found := false

	tag, sub, i := d.Tag(b, 0)
	if tag != tlwire.Map {
		t.Errorf("not a map object")
		return
	}

	for el := 0; sub == -1 || el < int(sub); el++ {
		if d.Break(b, &i) {
			break
		}

		var key []byte
		key, i = d.Bytes(b, i)

		tag, sem, vst := d.Tag(b, i)

		switch {
		case tag == tlwire.Semantic && sem == WireMessage && string(key) == KeyMessage:
			msg, i = d.Bytes(b, vst)
		case tag == tlwire.Semantic && sem == tlwire.Caller && string(key) == KeyCaller:
			pc, i = d.Caller(b, i)
			found = true
		default:
			i = d.Skip(b, i)
		}
	}

	assert.Equal(t, exists, found, "msg: %s", msg)

	if found {
		_, _, pcline := pc.NameFileLine()
		assert.Equal(t, line, pcline, "msg: %s", msg)
	}
}

func BenchmarkLoggerPrintw(b *testing.B) {
	b.ReportAllocs()

	l := New(ioutil.Discard)

	for i := 0; i < b.N; i++ {
		l.Printw("message", "a", i+1000, "b", i+1000, "c", "str")
	}
}

func BenchmarkLoggerPrintf(b *testing.B) {
	b.ReportAllocs()

	l := New(ioutil.Discard)

	for i := 0; i < b.N; i++ {
		l.Printf("message a %v b %v c %v", i+1000, i+1000, "str")
	}
}

func BenchmarkLoggerStartPrintwFinish(b *testing.B) {
	b.ReportAllocs()

	l := New(ioutil.Discard)

	for i := 0; i < b.N; i++ {
		tr := l.Start("span_name")
		tr.Printw("message", "a", i+1000, "b", i+1000, "c", "str")
		tr.Finish()
	}
}
