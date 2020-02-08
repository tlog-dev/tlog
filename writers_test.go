package tlog

import (
	"bytes"
	"encoding/hex"
	"io/ioutil"
	"regexp"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"

	"github.com/nikandfor/tlog/tlogpb"
)

func TestConsoleWriterAppendSegment(t *testing.T) {
	pref := []byte("prefix ")

	var w ConsoleWriter

	b := w.appendSegments(pref, 20, "path/to/file.go", '/')
	assert.Equal(t, "prefix path/to/file.go", string(b))

	b = w.appendSegments(pref, 12, "path/to/file.go", '/')
	assert.Equal(t, "prefix p/to/file.go", string(b))

	b = w.appendSegments(pref, 11, "path/to/file.go", '/')
	assert.Equal(t, "prefix p/t/file.go", string(b))

	b = w.appendSegments(pref, 10, "path/to/file.go", '/')
	assert.Equal(t, "prefix p/t/file.g", string(b))

	b = w.appendSegments(pref, 9, "path/to/file.go", '/')
	assert.Equal(t, "prefix p/t/file.", string(b))
}

func TestConsoleWriterBuildHeader(t *testing.T) {
	var w ConsoleWriter

	tm := time.Date(2019, 7, 7, 8, 19, 30, 100200300, time.UTC)
	loc := Caller(-1)

	w.f = Ldate | Ltime | Lmilliseconds | LUTC
	w.buildHeader(loc, tm)
	assert.Equal(t, "2019/07/07_08:19:30.100  ", string(w.buf))

	w.f = Ldate | Ltime | Lmicroseconds | LUTC
	w.buildHeader(loc, tm)
	assert.Equal(t, "2019/07/07_08:19:30.100200  ", string(w.buf))

	w.f = Llongfile
	w.buildHeader(loc, tm)
	ok, err := regexp.Match("(github.com/nikandfor/tlog/)?location.go:25  ", w.buf)
	assert.NoError(t, err)
	assert.True(t, ok, string(w.buf))

	w.f = Lshortfile
	w.Shortfile = 20
	w.buildHeader(loc, tm)
	assert.Equal(t, "location.go:25        ", string(w.buf))

	w.f = Lshortfile
	w.Shortfile = 10
	w.buildHeader(loc, tm)
	assert.Equal(t, "locatio:25  ", string(w.buf))

	w.f = Lfuncname
	w.Funcname = 10
	w.buildHeader(loc, tm)
	assert.Equal(t, "Caller      ", string(w.buf))

	w.f = Lfuncname
	w.Funcname = 4
	w.buildHeader(loc, tm)
	assert.Equal(t, "Call  ", string(w.buf))

	w.f = Lfuncname
	w.Funcname = 15
	w.buildHeader((&testt{}).testloc2(), tm)
	assert.Equal(t, "testloc2.func1   ", string(w.buf))

	w.f = Lfuncname
	w.Funcname = 12
	w.buildHeader((&testt{}).testloc2(), tm)
	assert.Equal(t, "testloc2.fu1  ", string(w.buf))

	w.f = Ltypefunc
	w.buildHeader(loc, tm)
	assert.Equal(t, "tlog.Caller  ", string(w.buf))

	w.buildHeader((&testt{}).testloc2(), tm)
	assert.Equal(t, "tlog.(*testt).testloc2.func1  ", string(w.buf))
}

func TestConsoleWriterSpans(t *testing.T) {
	tm := time.Date(2019, time.July, 7, 16, 31, 10, 0, time.Local)
	now = func() time.Time {
		tm = tm.Add(time.Second)
		return tm
	}
	randID = testRandID()

	w := NewConsoleWriter(ioutil.Discard, Ldate|Ltime|Lmilliseconds|Lspans|Lmessagespan)
	l := New(w)

	l.Labels(Labels{"a=b", "f"})

	assert.Equal(t, `2019/07/07_16:31:11.000  ________________  Labels: ["a=b" "f"]`+"\n", string(w.buf))

	tr := l.Start()

	assert.Equal(t, "2019/07/07_16:31:12.000  0194fdc2fa2ffcc0  Span started\n", string(w.buf))

	tr1 := l.Spawn(tr.ID)

	assert.Equal(t, "2019/07/07_16:31:13.000  6e4ff95ff662a5ee  Span spawned from 0194fdc2fa2ffcc0\n", string(w.buf))

	tr1.Printf("message")

	assert.Equal(t, "2019/07/07_16:31:14.000  6e4ff95ff662a5ee  message\n", string(w.buf))

	tr1.Finish()

	assert.Equal(t, "2019/07/07_16:31:15.000  6e4ff95ff662a5ee  Span finished - elapsed 2000.00ms\n", string(w.buf))

	tr.Finish()

	assert.Equal(t, "2019/07/07_16:31:16.000  0194fdc2fa2ffcc0  Span finished - elapsed 4000.00ms\n", string(w.buf))

	l.Printf("not traced message")

	assert.Equal(t, "2019/07/07_16:31:17.000  ________________  not traced message\n", string(w.buf))
}

func TestProtoAppendVarint(t *testing.T) {
	var pbuf proto.Buffer

	for i := uint(0); i < 64; i++ {
		b := appendVarint(nil, uint64(1<<i))

		pbuf.Reset()
		err := pbuf.EncodeVarint(uint64(1 << i))
		if !assert.NoError(t, err) {
			break
		}

		assert.Equal(t, pbuf.Bytes(), b, "%x", uint64(1<<i))
	}
}

func TestProtoWriter(t *testing.T) {
	var buf bytes.Buffer
	w := NewProtoWriter(&buf)
	var pbuf proto.Buffer

	w.Labels(Labels{"a", "b=c"})
	_ = pbuf.EncodeMessage(&tlogpb.Record{Labels: []string{"a", "b=c"}})
	assert.Equal(t, pbuf.Bytes(), buf.Bytes())
	t.Logf("Labels:\n%vexp:\n%v", hex.Dump(buf.Bytes()), hex.Dump(pbuf.Bytes()))

	buf.Reset()
	pbuf.Reset()

	loc := Caller(-1)
	name, file, line := loc.NameFileLine()

	id := ID{10, 20, 30, 40}

	w.Message(
		Message{
			Location: loc,
			Time:     time.Duration(2) << TimeReduction,
			Format:   "%v",
			Args:     []interface{}{4},
		},
		Span{ID: id},
	)
	_ = pbuf.EncodeMessage(&tlogpb.Record{Location: &tlogpb.Location{
		Pc:   int64(loc),
		Name: name,
		File: file,
		Line: int32(line),
	}})
	l := len(pbuf.Bytes())
	if l > buf.Len() {
		assert.Equal(t, pbuf.Bytes(), buf.Bytes())
		return
	}

	assert.Equal(t, pbuf.Bytes(), buf.Bytes()[:l])
	t.Logf("Location:\n%vexp:\n%v", hex.Dump(buf.Bytes()[:l]), hex.Dump(pbuf.Bytes()))

	_ = pbuf.EncodeMessage(&tlogpb.Record{Msg: &tlogpb.Message{
		Span:     id[:],
		Location: int64(loc),
		Time:     2,
		Text:     "4",
	}})
	assert.Equal(t, pbuf.Bytes()[l:], buf.Bytes()[l:])
	t.Logf("Message:\n%vexp:\n%v", hex.Dump(buf.Bytes()[l:]), hex.Dump(pbuf.Bytes()[l:]))

	buf.Reset()
	pbuf.Reset()
	delete(w.ls, loc)

	id = ID{5, 15, 25, 35}
	par := ID{4, 14, 24, 34}

	w.SpanStarted(
		Span{
			ID:      id,
			Started: time.Unix(0, 2<<TimeReduction),
		},
		par,
		loc,
	)
	_ = pbuf.EncodeMessage(&tlogpb.Record{Location: &tlogpb.Location{
		Pc:   int64(loc),
		Name: name,
		File: file,
		Line: int32(line),
	}})
	l = len(pbuf.Bytes())
	if l > buf.Len() {
		assert.Equal(t, pbuf.Bytes(), buf.Bytes())
		return
	}

	_ = pbuf.EncodeMessage(&tlogpb.Record{SpanStart: &tlogpb.SpanStart{
		Id:       id[:],
		Parent:   par[:],
		Location: int64(loc),
		Started:  2,
	}})
	assert.Equal(t, pbuf.Bytes()[l:], buf.Bytes()[l:])
	t.Logf("SpanStart:\n%vexp:\n%v", hex.Dump(buf.Bytes()[l:]), hex.Dump(pbuf.Bytes()[l:]))

	buf.Reset()
	pbuf.Reset()

	w.SpanFinished(
		Span{
			ID: id,
		},
		time.Second,
	)
	_ = pbuf.EncodeMessage(&tlogpb.Record{SpanFinish: &tlogpb.SpanFinish{
		Id:      id[:],
		Elapsed: time.Second.Nanoseconds() >> TimeReduction,
	}})
	assert.Equal(t, pbuf.Bytes(), buf.Bytes())
	t.Logf("SpanFinish:\n%vexp:\n%v", hex.Dump(buf.Bytes()), hex.Dump(pbuf.Bytes()))

	buf.Reset()
	pbuf.Reset()
}

func TestLockedWriter(t *testing.T) {
	l := New(NewLockedWriter(Discard{}))

	l.Labels(Labels{"a", "b"})
	tr := l.Start()
	tr.Printf("message: %v", 2)
	tr.Finish()
}

func TestTeeWriter(t *testing.T) {
	var buf1, buf2 bytes.Buffer

	w1 := NewJSONWriter(&buf1)
	w2 := NewJSONWriter(&buf2)

	w := NewTeeWriter(w1, w2)

	w.Labels(Labels{"a=b", "f"})
	w.Message(Message{Format: "msg"}, Span{})
	w.SpanStarted(Span{ID: ID{100}, Started: time.Date(2019, 7, 6, 10, 18, 32, 0, time.UTC)}, z, 0)
	w.SpanFinished(Span{ID: ID{100}}, time.Second)

	assert.Equal(t, `{"L":["a=b","f"]}
{"l":{"p":0,"f":"","l":0,"n":""}}
{"m":{"l":0,"t":0,"m":"msg"}}
{"s":{"i":"64000000000000000000000000000000","l":0,"s":24412629875000000}}
{"f":{"i":"64000000000000000000000000000000","e":15625000}}
`, buf1.String())
	assert.Equal(t, buf1.String(), buf2.String())
}

func TestNewTeeWriter(t *testing.T) {
	a := NewTeeWriter(Discard{})
	b := NewTeeWriter(Discard{})
	c := NewTeeWriter(Discard{}, Discard{})

	d := NewTeeWriter(a, b, c, Discard{})

	assert.Len(t, d, 5)
}

func BenchmarkWriterConsoleDetailedMessage(b *testing.B) {
	w := NewConsoleWriter(ioutil.Discard, LdetFlags)

	l := Caller(0)

	for i := 0; i < b.N; i++ {
		w.Message(Message{
			Location: l,
			Time:     time.Second,
			Format:   "some message",
		}, Span{})
	}
}

func BenchmarkWriterJSONMessage(b *testing.B) {
	w := NewJSONWriter(ioutil.Discard)

	l := Caller(0)

	for i := 0; i < b.N; i++ {
		w.Message(Message{
			Location: l,
			Time:     time.Second,
			Format:   "some message",
		}, Span{})
	}
}

func BenchmarkWriterProtoMessage(b *testing.B) {
	w := NewProtoWriter(ioutil.Discard)

	l := Caller(0)

	for i := 0; i < b.N; i++ {
		w.Message(Message{
			Location: l,
			Time:     time.Second,
			Format:   "some message",
		}, Span{})
	}
}
