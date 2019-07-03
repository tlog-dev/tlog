package tlog

import (
	"bytes"
	"io"
	"math/rand"
	"testing"
	"time"

	"github.com/nikandfor/json"
	"github.com/stretchr/testify/assert"
)

func TestJSONReader(t *testing.T) {
	defer func(old func() time.Time) {
		now = old
	}(now)
	tm := time.Date(2019, time.July, 1, 10, 54, 10, 0, time.Local)
	start := tm
	now = func() time.Time {
		r := tm
		tm = tm.Add(time.Second)
		return r
	}
	rnd = rand.New(rand.NewSource(0))

	var buf bytes.Buffer

	jw := json.NewStreamWriter(&buf)
	w := NewJSONWriter(jw)
	l := NewLogger(w)

	ls := Labels{"a=b", "c", "d=e"}
	w.Labels(ls)

	l.Printf("msg1 %d", 3)

	tr := l.Start()
	tr.Printf("msg2 %d", 5)

	tr1 := l.Spawn(tr.ID)

	tr1.Printf("msg3 %d", 7)

	tr1.Finish()

	tr.Finish()

	jw.Flush()

	// read

	// Printf("\n%s", buf.Bytes())

	jr := json.NewReader(&buf)
	r := NewJSONReader(jr)

	v := r.Read()
	assert.Equal(t, ls, v)

	v = r.Read()
	li, ok := v.(*LocationInfo)
	if assert.True(t, ok) {
		//	assert.Equal(t, "github.com/nikandfor/tlog/reader_test.go", li.File)
		assert.Equal(t, "tlog.TestJSONReader", li.Func)
	}

	v = r.Read()
	msg, ok := v.(*Message)
	if assert.True(t, ok) {
		assert.Equal(t, "msg1 3", msg.Format)
		assert.Equal(t, li.PC, msg.Location)
		assert.Equal(t, start.Add(0*time.Second), msg.AbsTime())
		assert.Nil(t, msg.Args)
	}

	v = r.Read()
	sli, ok := v.(*LocationInfo)
	if assert.True(t, ok) {
		//	assert.Equal(t, "github.com/nikandfor/tlog/reader_test.go", li.File)
		assert.Equal(t, "tlog.TestJSONReader", li.Func)
	}

	v = r.Read()
	sp, ok := v.(*Span)
	if assert.True(t, ok) {
		assert.Equal(t, sli.PC, sp.Location)
		assert.Equal(t, ID(0x78fc2ffac2fd9401), sp.ID, "have %v", sp.ID)
		assert.Equal(t, start.Add(time.Second), sp.Started)
	}

	v = r.Read()
	li, ok = v.(*LocationInfo)
	if assert.True(t, ok) {
		//	assert.Equal(t, "github.com/nikandfor/tlog/reader_test.go", li.File)
		assert.Equal(t, "tlog.TestJSONReader", li.Func)
	}

	v = r.Read()
	msg, ok = v.(*Message)
	if assert.True(t, ok) {
		assert.Equal(t, "msg2 5", msg.Format)
		assert.Equal(t, li.PC, msg.Location)
		assert.Equal(t, time.Second, msg.Time)
		assert.True(t, len(msg.Args) == 1 && msg.Args[0] == sp.ID)
	}

	v = r.Read()
	sp2, ok := v.(*Span)
	if assert.True(t, ok) {
		assert.Equal(t, sli.PC, sp2.Location)
		assert.Equal(t, ID(0x1f5b0412ffd341c0), sp2.ID, "have %v", sp2.ID)
		assert.Equal(t, sp.ID, sp2.Parent)
		assert.Equal(t, start.Add(3*time.Second), sp2.Started)
	}

	v = r.Read()
	li, ok = v.(*LocationInfo)
	if assert.True(t, ok) {
		//	assert.Equal(t, "github.com/nikandfor/tlog/reader_test.go", li.File)
		assert.Equal(t, "tlog.TestJSONReader", li.Func)
	}

	v = r.Read()
	msg, ok = v.(*Message)
	if assert.True(t, ok) {
		assert.Equal(t, "msg3 7", msg.Format)
		assert.Equal(t, li.PC, msg.Location)
		assert.Equal(t, time.Second, msg.Time)
		assert.True(t, len(msg.Args) == 1 && msg.Args[0] == sp2.ID)
	}

	v = r.Read()
	f, ok := v.(*SpanFinish)
	if assert.True(t, ok) {
		assert.Equal(t, f.ID, sp2.ID)
		assert.Equal(t, 2*time.Second, f.Elapsed)
	}

	v = r.Read()
	f, ok = v.(*SpanFinish)
	if assert.True(t, ok) {
		assert.Equal(t, f.ID, sp.ID)
		assert.Equal(t, 5*time.Second, f.Elapsed)
	}

	v = r.Read()
	assert.Equal(t, io.EOF, v)
}
