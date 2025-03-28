package tlog

import (
	"io"
	"testing"
	"time"

	"github.com/nikandfor/assert"
	"nikand.dev/go/hacked/low"
	"tlog.app/go/loc"

	"tlog.app/go/tlog/tlwire"
)

type MultiWriter []io.Writer

func TestConsoleLocations(t *testing.T) {
	var buf, raw low.Buf

	w := NewConsoleWriter(&buf, 0)
	l := New(MultiWriter{&raw, w})

	w.PadEmptyMessage = false

	c := loc.Caller(-1)
	cc := loc.Callers(-1, 2)

	{
		name, file, line := c.NameFileLine()
		t.Logf("caller: %v %v %v", name, file, line)

		for _, pc := range cc {
			name, file, line := pc.NameFileLine()
			t.Logf("callers: %v %v %v", name, file, line)
		}
	}

	_ = l.Event("caller", c)
	assert.Equal(t, "caller=location.go:24\n", string(buf))

	buf = buf[:0]

	_ = l.Event("callers", cc)
	assert.Equal(t, "callers=[location.go:71 console_test.go:26]\n", string(buf))

	t.Logf("dump:\n%v", tlwire.Dump(raw))
}

func (w MultiWriter) Write(p []byte) (n int, err error) {
	for i, w := range w {
		m, e := w.Write(p)

		if i == 0 {
			n = m
		}

		if err == nil {
			err = e
		}
	}

	return
}

func TestConsole(tb *testing.T) {
	var e tlwire.Encoder
	var b, jb low.Buf

	j := NewConsoleWriter(&jb, 0)

	b = e.AppendInt(b[:0], 5)
	jb, _ = j.ConvertValue(jb[:0], b, 0, 0)
	assert.Equal(tb, low.Buf("5"), jb)

	b = e.AppendInt(b[:0], -5)
	jb, _ = j.ConvertValue(jb[:0], b, 0, 0)
	assert.Equal(tb, low.Buf("-5"), jb)
}

func TestAppendDuration(t *testing.T) {
	w := NewConsoleWriter(nil, 0)

	t.Logf("%-10s", w.AppendDuration(nil, 0))

	for _, days := range []int{0, 2} {
		for _, h := range []int{0, 12} {
			for _, m := range []int{0, 2} {
				for _, s := range []int{0, 36} {
					d := time.Hour*time.Duration(24*days+h) +
						time.Minute*time.Duration(m) +
						time.Second*time.Duration(s)

					t.Logf("%-10s is %v", w.AppendDuration(nil, d), d)
				}
			}
		}
	}

	for d := time.Nanosecond; d < 100*time.Second; d *= 7 {
		t.Logf("%-10s is %v", w.AppendDuration(nil, d), d)
	}

	for f := float64(1); f < float64(200*time.Microsecond); f *= 1.378 {
		d := time.Duration(f)
		t.Logf("%-10s is %v", w.AppendDuration(nil, d), d)
	}

	d := 99999 * time.Microsecond
	t.Logf("%-10s is %v", w.AppendDuration(nil, d), d)
	d = 999999 * time.Microsecond
	t.Logf("%-10s is %v", w.AppendDuration(nil, d), d)
}

func BenchmarkConsolePrintw(b *testing.B) {
	b.ReportAllocs()

	w := NewConsoleWriter(io.Discard, LdetFlags)
	l := New(w)

	for i := range b.N {
		l.Printw("message", "a", i+1000, "b", i+1000, "c", "str")
	}
}

func BenchmarkConsoleStartPrintwFinish(b *testing.B) {
	b.ReportAllocs()

	w := NewConsoleWriter(io.Discard, LdetFlags)
	l := New(w)

	for i := range b.N {
		tr := l.Start("span_name")
		tr.Printw("message", "a", i+1000, "b", i+1000, "c", "str")
		tr.Finish()
	}
}
