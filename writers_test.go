package tlog

import (
	"bytes"
	"encoding/hex"
	"io/ioutil"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"

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
	w.buildHeader(loc, tm.UnixNano())
	assert.Equal(t, "2019/07/07_08:19:30.100  ", string(w.buf))

	w.f = Ldate | Ltime | Lmicroseconds | LUTC
	w.buildHeader(loc, tm.UnixNano())
	assert.Equal(t, "2019/07/07_08:19:30.100200  ", string(w.buf))

	w.f = Llongfile
	w.buildHeader(loc, tm.UnixNano())
	ok, err := regexp.Match("(github.com/nikandfor/tlog/)?location.go:25  ", w.buf)
	assert.NoError(t, err)
	assert.True(t, ok, string(w.buf))

	w.f = Lshortfile
	w.Shortfile = 20
	w.buildHeader(loc, tm.UnixNano())
	assert.Equal(t, "location.go:25        ", string(w.buf))

	w.f = Lshortfile
	w.Shortfile = 10
	w.buildHeader(loc, tm.UnixNano())
	assert.Equal(t, "locatio:25  ", string(w.buf))

	w.f = Lfuncname
	w.Funcname = 10
	w.buildHeader(loc, tm.UnixNano())
	assert.Equal(t, "Caller      ", string(w.buf))

	w.f = Lfuncname
	w.Funcname = 4
	w.buildHeader(loc, tm.UnixNano())
	assert.Equal(t, "Call  ", string(w.buf))

	w.f = Lfuncname
	w.Funcname = 15
	w.buildHeader((&testt{}).testloc2(), tm.UnixNano())
	assert.Equal(t, "testloc2.func1   ", string(w.buf))

	w.f = Lfuncname
	w.Funcname = 12
	w.buildHeader((&testt{}).testloc2(), tm.UnixNano())
	assert.Equal(t, "testloc2.fu1  ", string(w.buf))

	w.f = Ltypefunc
	w.buildHeader(loc, tm.UnixNano())
	assert.Equal(t, "tlog.Caller  ", string(w.buf))

	w.buildHeader((&testt{}).testloc2(), tm.UnixNano())
	assert.Equal(t, "tlog.(*testt).testloc2.func1  ", string(w.buf))
}

func TestConsoleWriterSpans(t *testing.T) {
	tm := time.Date(2019, time.July, 7, 16, 31, 10, 0, time.Local)
	now = testNow(&tm)

	w := NewConsoleWriter(ioutil.Discard, Ldate|Ltime|Lmilliseconds|Lspans|Lmessagespan)
	l := New(w)
	l.randID = testRandID()

	l.SetLabels(Labels{"a=b", "f"})

	assert.Equal(t, `2019/07/07_16:31:11.000  ________________  Labels: ["a=b" "f"]`+"\n", string(w.buf))

	tr := l.Start()

	assert.Equal(t, "2019/07/07_16:31:12.000  0194fdc2fa2ffcc0  Span started\n", string(w.buf))

	tr.SetLabels(Labels{"a=c", "c=d", "g"})

	assert.Equal(t, `2019/07/07_16:31:13.000  0194fdc2fa2ffcc0  Labels: ["a=c" "c=d" "g"]`+"\n", string(w.buf))

	tr1 := l.Spawn(tr.ID)

	assert.Equal(t, "2019/07/07_16:31:14.000  6e4ff95ff662a5ee  Span spawned from 0194fdc2fa2ffcc0\n", string(w.buf))

	tr1.Printf("message")

	assert.Equal(t, "2019/07/07_16:31:15.000  6e4ff95ff662a5ee  message\n", string(w.buf))

	tr1.Finish()

	assert.Equal(t, "2019/07/07_16:31:17.000  6e4ff95ff662a5ee  Span finished - elapsed 2000.00ms\n", string(w.buf))

	tr.Finish()

	assert.Equal(t, "2019/07/07_16:31:19.000  0194fdc2fa2ffcc0  Span finished - elapsed 6000.00ms\n", string(w.buf))

	l.Printf("not traced message")

	assert.Equal(t, "2019/07/07_16:31:20.000  ________________  not traced message\n", string(w.buf))
}

func TestProtoAppendVarint(t *testing.T) {
	var pbuf []byte

	for i := uint(0); i < 64; i++ {
		b := appendVarint(nil, uint64(1<<i))

		pbuf = protowire.AppendVarint(pbuf[:0], uint64(1<<i))

		assert.Equal(t, pbuf, b, "%x", uint64(1<<i))
	}
}

func TestProtoAppendTagVarint(t *testing.T) {
	var pbuf []byte

	for i := uint(0); i < 64; i++ {
		b := appendTagVarint(nil, 0x77, uint64(1<<i))

		pbuf = protowire.AppendVarint(pbuf[:0], 0x77)
		pbuf = protowire.AppendVarint(pbuf, uint64(1<<i))

		assert.Equal(t, pbuf, b, "%x", uint64(1<<i))
	}
}

func TestProtoWriter(t *testing.T) {
	var buf bytes.Buffer
	w := NewProtoWriter(&buf)
	var pbuf []byte

	_ = w.Labels(Labels{"a", "b=c"}, ID{})
	pbuf = encode(pbuf[:0], &tlogpb.Record{Labels: &tlogpb.Labels{Labels: []string{"a", "b=c"}}})
	assert.Equal(t, pbuf, buf.Bytes())
	t.Logf("Labels:\n%vexp:\n%v", hex.Dump(buf.Bytes()), hex.Dump(pbuf))

	buf.Reset()
	pbuf = pbuf[:0]

	loc := Caller(-1)
	name, file, line := loc.NameFileLine()

	id := ID{10, 20, 30, 40}

	_ = w.Message(
		Message{
			Location: loc,
			Time:     2,
			Format:   "%v",
			Args:     []interface{}{4},
		},
		id,
	)
	pbuf = encode(pbuf, &tlogpb.Record{Location: &tlogpb.Location{
		Pc:    int64(loc),
		Entry: int64(loc.Entry()),
		Name:  name,
		File:  file,
		Line:  int32(line),
	}})
	l := len(pbuf)
	if l > buf.Len() {
		assert.Equal(t, pbuf, buf.Bytes())
		return
	}

	assert.Equal(t, pbuf, buf.Bytes()[:l])
	t.Logf("Location:\n%vexp:\n%v", hex.Dump(buf.Bytes()[:l]), hex.Dump(pbuf))

	pbuf = encode(pbuf, &tlogpb.Record{Message: &tlogpb.Message{
		Span:     id[:],
		Location: int64(loc),
		Time:     2,
		Text:     "4",
	}})
	assert.Equal(t, pbuf[l:], buf.Bytes()[l:])
	t.Logf("Message:\n%vexp:\n%v", hex.Dump(buf.Bytes()[l:]), hex.Dump(pbuf[l:]))

	buf.Reset()
	pbuf = pbuf[:0]
	delete(w.ls, loc)

	id = ID{5, 15, 25, 35}
	par := ID{4, 14, 24, 34}

	_ = w.SpanStarted(
		id,
		par,
		2,
		loc,
	)
	pbuf = encode(pbuf, &tlogpb.Record{Location: &tlogpb.Location{
		Pc:    int64(loc),
		Entry: int64(loc.Entry()),
		Name:  name,
		File:  file,
		Line:  int32(line),
	}})
	l = len(pbuf)
	if l > buf.Len() {
		assert.Equal(t, pbuf, buf.Bytes())
		return
	}

	pbuf = encode(pbuf, &tlogpb.Record{SpanStart: &tlogpb.SpanStart{
		Id:       id[:],
		Parent:   par[:],
		Location: int64(loc),
		Started:  2,
	}})
	assert.Equal(t, pbuf[l:], buf.Bytes()[l:])
	t.Logf("SpanStart:\n%vexp:\n%v", hex.Dump(buf.Bytes()[l:]), hex.Dump(pbuf[l:]))

	buf.Reset()
	pbuf = pbuf[:0]

	_ = w.Labels(Labels{"b=d", "d"}, id)

	pbuf = encode(pbuf, &tlogpb.Record{Labels: &tlogpb.Labels{
		Span:   id[:],
		Labels: []string{"b=d", "d"},
	}})

	assert.Equal(t, pbuf, buf.Bytes())
	t.Logf("Span Labels:\n%vexp:\n%v", hex.Dump(buf.Bytes()), hex.Dump(pbuf))

	buf.Reset()
	pbuf = pbuf[:0]

	_ = w.SpanFinished(
		id,
		time.Second.Nanoseconds(),
	)
	pbuf = encode(pbuf, &tlogpb.Record{SpanFinish: &tlogpb.SpanFinish{
		Id:      id[:],
		Elapsed: time.Second.Nanoseconds(),
	}})
	assert.Equal(t, pbuf, buf.Bytes())
	t.Logf("SpanFinish:\n%vexp:\n%v", hex.Dump(buf.Bytes()), hex.Dump(pbuf))

	buf.Reset()
	pbuf = pbuf[:0]

	_ = w.Message(
		Message{
			Location: loc,
			Time:     2,
			Format:   string(make([]byte, 1000)),
		},
		id,
	)
	pbuf = encode(pbuf, &tlogpb.Record{Message: &tlogpb.Message{
		Span:     id[:],
		Location: int64(loc),
		Time:     2,
		Text:     string(make([]byte, 1000)),
	}})
	if !assert.Equal(t, pbuf, buf.Bytes()) {
		t.Logf("Message:\n%vexp:\n%v", hex.Dump(buf.Bytes()), hex.Dump(pbuf))
	}

	buf.Reset()
	pbuf = pbuf[:0]

	// metric info
	_ = w.Metric(
		Metric{
			Name:   "op_name_metric",
			Labels: []string{"const=1", "cc=2"},
			Help:   "help message",
			Type:   "type",
			Meta:   true,
		},
		ID{},
	)
	pbuf = encode(pbuf, &tlogpb.Record{Metric: &tlogpb.Metric{
		Name:   "op_name_metric",
		Labels: []string{"const=1", "cc=2"},
		Help:   "help message",
		Type:   "type",
	}})
	if !assert.Equal(t, pbuf, buf.Bytes()) {
		t.Logf("Metric:\n%vexp:\n%v", hex.Dump(buf.Bytes()), hex.Dump(pbuf))
	}

	buf.Reset()
	pbuf = pbuf[:0]

	// metric itself
	var h uintptr
	for _, s := range []string{"op_name_metric", "m=1", "mm=2"} {
		h = strhash(&s, h)
	}

	_ = w.Metric(
		Metric{
			Name:   "op_name_metric",
			Value:  123.456,
			Labels: []string{"m=1", "mm=2"},
		},
		id,
	)
	pbuf = encode(pbuf, &tlogpb.Record{Metric: &tlogpb.Metric{
		Span:   id[:],
		Hash:   uint64(h),
		Name:   "op_name_metric",
		Value:  123.456,
		Labels: []string{"m=1", "mm=2"},
	}})
	if !assert.Equal(t, pbuf, buf.Bytes()) {
		t.Logf("Metric:\n%vexp:\n%v", hex.Dump(buf.Bytes()), hex.Dump(pbuf))
	}

	buf.Reset()
	pbuf = pbuf[:0]

	// second time is encoded more compact
	_ = w.Metric(
		Metric{
			Name:   "op_name_metric",
			Value:  111.222,
			Labels: []string{"m=1", "mm=2"},
		},
		id,
	)
	pbuf = encode(pbuf, &tlogpb.Record{Metric: &tlogpb.Metric{
		Span:  id[:],
		Hash:  uint64(h),
		Value: 111.222,
	}})
	if !assert.Equal(t, pbuf, buf.Bytes()) {
		t.Logf("Metric2:\n%vexp:\n%v", hex.Dump(buf.Bytes()), hex.Dump(pbuf))
	}
}

func TestLockedWriter(t *testing.T) {
	l := New(NewLockedWriter(Discard))

	l.SetLabels(Labels{"a", "b"})
	tr := l.Start()
	tr.Printf("message: %v", 2)
	tr.Finish()
}

func TestTeeWriter(t *testing.T) {
	var buf1, buf2 bytes.Buffer

	w1 := NewJSONWriter(&buf1)
	w2 := NewJSONWriter(&buf2)

	w := NewTeeWriter(w1, w2)

	fe := Funcentry(-1)
	loc := Caller(-1)

	_ = w.Labels(Labels{"a=b", "f"}, ID{})
	_ = w.Message(Message{Location: loc, Format: "msg", Time: 1}, ID{})
	_ = w.SpanStarted(ID{100}, ID{}, time.Date(2019, 7, 6, 10, 18, 32, 0, time.UTC).UnixNano(), fe)
	_ = w.SpanFinished(ID{100}, time.Second.Nanoseconds())

	re := `{"L":{"L":\["a=b","f"\]}}
{"l":{"p":\d+,"e":\d+,"f":"[\w.-/]*location.go","l":25,"n":"github.com/nikandfor/tlog.Caller"}}
{"m":{"t":1,"l":\d+,"m":"msg"}}
{"l":{"p":\d+,"e":\d+,"f":"[\w.-/]*location.go","l":32,"n":"github.com/nikandfor/tlog.Funcentry"}}
{"s":{"i":"64000000000000000000000000000000","s":1562408312000000000,"l":\d+}}
{"f":{"i":"64000000000000000000000000000000","e":1000000000}}
`
	ok, err := regexp.Match(re, buf1.Bytes())
	assert.NoError(t, err)
	assert.True(t, ok, "expected:\n%vactual:\n%v", re, buf1.String())

	assert.Equal(t, buf1.String(), buf2.String())
}

func TestNewTeeWriter(t *testing.T) {
	a := NewTeeWriter(Discard)
	b := NewTeeWriter(Discard)
	c := NewTeeWriter(Discard, Discard)

	d := NewTeeWriter(a, b, c, Discard)

	assert.Len(t, d, 5)
}

func BenchmarkWriterConsoleDetailedMessage(b *testing.B) {
	b.ReportAllocs()

	w := NewConsoleWriter(ioutil.Discard, LdetFlags)

	l := Caller(0)

	for i := 0; i < b.N; i++ {
		_ = w.Message(Message{
			Location: l,
			Time:     1,
			Format:   "some message",
		}, ID{})
	}
}

func BenchmarkWriterJSONMessage(b *testing.B) {
	b.ReportAllocs()

	w := NewJSONWriter(ioutil.Discard)

	l := Caller(0)

	for i := 0; i < b.N; i++ {
		_ = w.Message(Message{
			Location: l,
			Time:     1,
			Format:   "some message",
		}, ID{1, 2, 3, 4, 5, 6, 7, 8})
	}
}

func BenchmarkWriterJSONMetric(b *testing.B) {
	b.ReportAllocs()

	w := NewJSONWriter(ioutil.Discard)

	for i := 0; i < b.N; i++ {
		_ = w.Metric(Metric{
			Name:  "some_metric",
			Value: 123.456,
		}, ID{1, 2, 3, 4, 5, 6, 7, 8})
	}
}

func BenchmarkWriterProtoMessage(b *testing.B) {
	b.ReportAllocs()

	w := NewProtoWriter(ioutil.Discard)

	l := Caller(0)

	for i := 0; i < b.N; i++ {
		_ = w.Message(Message{
			Location: l,
			Time:     1,
			Format:   "some message",
		}, ID{})
	}
}

func encode(pbuf []byte, m proto.Message) []byte {
	pbuf = protowire.AppendVarint(pbuf, uint64(proto.Size(m)))

	var err error
	pbuf, err = proto.MarshalOptions{
		Deterministic: true,
	}.MarshalAppend(pbuf, m)

	if err != nil {
		panic(err)
	}

	return pbuf
}
