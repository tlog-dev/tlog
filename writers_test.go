package tlog

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"

	"github.com/nikandfor/tlog/tlogpb"
)

func TestConsoleWriterBuildHeader(t *testing.T) {
	var w ConsoleWriter
	var b bufWriter

	tm := time.Date(2019, 7, 7, 8, 19, 30, 100200300, time.UTC)
	loc := Caller(-1)

	w.f = Ldate | Ltime | Lmilliseconds | LUTC
	b = w.buildHeader(b[:0], 0, tm.UnixNano(), loc)
	assert.Equal(t, "2019-07-07_08:19:30.100  ", string(b))

	w.f = Ldate | Ltime | Lmicroseconds | LUTC
	b = w.buildHeader(b[:0], 0, tm.UnixNano(), loc)
	assert.Equal(t, "2019-07-07_08:19:30.100200  ", string(b))

	w.f = Llongfile
	b = w.buildHeader(b[:0], 0, tm.UnixNano(), loc)
	ok, err := regexp.Match("(github.com/nikandfor/tlog/)?location.go:25  ", b)
	assert.NoError(t, err)
	assert.True(t, ok, string(b))

	w.f = Lshortfile
	w.Shortfile = 20
	b = w.buildHeader(b[:0], 0, tm.UnixNano(), loc)
	assert.Equal(t, "location.go:25        ", string(b))

	w.f = Lshortfile
	w.Shortfile = 10
	b = w.buildHeader(b[:0], 0, tm.UnixNano(), loc)
	assert.Equal(t, "locatio:25  ", string(b))

	w.f = Lfuncname
	w.Funcname = 10
	b = w.buildHeader(b[:0], 0, tm.UnixNano(), loc)
	assert.Equal(t, "Caller      ", string(b))

	w.f = Lfuncname
	w.Funcname = 4
	b = w.buildHeader(b[:0], 0, tm.UnixNano(), loc)
	assert.Equal(t, "Call  ", string(b))

	w.f = Lfuncname
	w.Funcname = 15
	b = w.buildHeader(b[:0], 0, tm.UnixNano(), (&testt{}).testloc2())
	assert.Equal(t, "testloc2.func1   ", string(b))

	w.f = Lfuncname
	w.Funcname = 12
	b = w.buildHeader(b[:0], 0, tm.UnixNano(), (&testt{}).testloc2())
	assert.Equal(t, "testloc2.fu1  ", string(b))

	w.f = Ltypefunc
	b = w.buildHeader(b[:0], 0, tm.UnixNano(), loc)
	assert.Equal(t, "tlog.Caller  ", string(b))

	b = w.buildHeader(b[:0], 0, tm.UnixNano(), (&testt{}).testloc2())
	assert.Equal(t, "tlog.(*testt).testloc2.func1  ", string(b))
}

func TestConsoleWriterSpans(t *testing.T) {
	tm := time.Date(2019, time.July, 7, 16, 31, 10, 0, time.Local)
	now = testNow(&tm)

	var b bufWriter

	w := NewConsoleWriter(&b, Ldate|Ltime|Lmilliseconds|Lspans|Lmessagespan)
	l := New(w)
	l.NewID = testRandID()

	l.SetLabels(Labels{"a=b", "f"})

	assert.Equal(t, `2019-07-07_16:31:11.000  ________  Labels: a=b f`+"\n", string(b))

	b = b[:0]

	tr := l.Start()

	assert.Equal(t, "2019-07-07_16:31:12.000  0194fdc2  Span started\n", string(b))

	b = b[:0]

	tr.SetLabels(Labels{"a=c", "c=d", "g"})

	assert.Equal(t, `2019-07-07_16:31:13.000  0194fdc2  Labels: a=c c=d g`+"\n", string(b))

	b = b[:0]

	tr1 := l.Spawn(tr.ID)

	assert.Equal(t, "2019-07-07_16:31:14.000  6e4ff95f  Span spawned from 0194fdc2\n", string(b))

	b = b[:0]

	tr1.Printf("message")

	assert.Equal(t, "2019-07-07_16:31:15.000  6e4ff95f  message\n", string(b))

	b = b[:0]

	tr1.Finish()

	assert.Equal(t, "2019-07-07_16:31:17.000  6e4ff95f  Span finished - elapsed 2000.00ms\n", string(b))

	b = b[:0]

	tr.Finish()

	assert.Equal(t, "2019-07-07_16:31:19.000  0194fdc2  Span finished - elapsed 6000.00ms\n", string(b))

	b = b[:0]

	l.Printf("not traced message")

	assert.Equal(t, "2019-07-07_16:31:20.000  ________  not traced message\n", string(b))
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
			PC:   loc,
			Time: 2,
			Text: "4",
		},
		id,
	)
	pbuf = encode(pbuf, &tlogpb.Record{Frame: &tlogpb.Frame{
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
		Span: id[:],
		Pc:   int64(loc),
		Time: 2,
		Text: "4",
	}})
	assert.Equal(t, pbuf[l:], buf.Bytes()[l:])
	t.Logf("Message:\n%vexp:\n%v", hex.Dump(buf.Bytes()[l:]), hex.Dump(pbuf[l:]))

	buf.Reset()
	pbuf = pbuf[:0]
	delete(w.ls, loc)

	id = ID{5, 15, 25, 35}
	par := ID{4, 14, 24, 34}

	// SpanStarted
	_ = w.SpanStarted(SpanStart{
		ID:        id,
		Parent:    par,
		StartedAt: 2,
		PC:        loc,
	})
	pbuf = encode(pbuf, &tlogpb.Record{Frame: &tlogpb.Frame{
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
		Id:        id[:],
		Parent:    par[:],
		Pc:        int64(loc),
		StartedAt: 2,
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

	// SpanFinished
	_ = w.SpanFinished(SpanFinish{
		ID:      id,
		Elapsed: time.Second.Nanoseconds(),
	})
	pbuf = encode(pbuf, &tlogpb.Record{SpanFinish: &tlogpb.SpanFinish{
		Id:      id[:],
		Elapsed: time.Second.Nanoseconds(),
	}})
	assert.Equal(t, pbuf, buf.Bytes())
	t.Logf("SpanFinish:\n%vexp:\n%v", hex.Dump(buf.Bytes()), hex.Dump(pbuf))

	buf.Reset()
	pbuf = pbuf[:0]

	// Message
	_ = w.Message(
		Message{
			PC:    loc,
			Time:  2,
			Level: ErrorLevel,
			Text:  string(make([]byte, 1000)),
		},
		id,
	)
	pbuf = encode(pbuf, &tlogpb.Record{Message: &tlogpb.Message{
		Span:  id[:],
		Pc:    int64(loc),
		Time:  2,
		Level: int32(ErrorLevel),
		Text:  string(make([]byte, 1000)),
	}})
	if !assert.Equal(t, pbuf, buf.Bytes()) {
		t.Logf("Message:\n%vexp:\n%v", hex.Dump(buf.Bytes()), hex.Dump(pbuf))
	}

	buf.Reset()
	pbuf = pbuf[:0]

	// Metric with Attributes
	_ = w.Message(
		Message{
			Text: "text",
			Attrs: Attrs{
				{"id", id},
				{"int", 8},
				{"uint", uint(10)},
				{"float", 3.3},
				{"str", "string"},
				{"undef", Message{}},
			},
		},
		ID{},
	)
	pbuf = encode(pbuf, &tlogpb.Record{Message: &tlogpb.Message{
		Text: "text",
		Attrs: []*tlogpb.Attr{
			{Name: "id", Type: 'd', Bytes: id[:]},
			{Name: "int", Type: 'i', Int: 8},
			{Name: "uint", Type: 'u', Uint: 10},
			{Name: "float", Type: 'f', Float: 3.3},
			{Name: "str", Type: 's', Str: "string"},
			{Name: "undef", Type: '?', Str: "tlog.Message"},
		},
	}})
	if !assert.Equal(t, pbuf, buf.Bytes()) {
		t.Logf("Message:\n%vexp:\n%v", hex.Dump(buf.Bytes()), hex.Dump(pbuf))
	}

	buf.Reset()
	pbuf = pbuf[:0]

	// metric info
	_ = w.Meta(
		Meta{
			Type: "metric_desc",
			Data: Labels{
				"name=" + "op_name_metric",
				"type=" + "type",
				"help=" + "help message",
				"labels",
				"const=1", "cc=2",
			},
		},
	)
	pbuf = encode(pbuf, &tlogpb.Record{Meta: &tlogpb.Meta{
		Type: "metric_desc",
		Data: []string{
			"name=op_name_metric",
			"type=type",
			"help=help message",
			"labels",
			"const=1", "cc=2",
		},
	}})
	if !assert.Equal(t, pbuf, buf.Bytes()) {
		t.Logf("Metric:\n%vexp:\n%v", hex.Dump(buf.Bytes()), hex.Dump(pbuf))
	}

	buf.Reset()
	pbuf = pbuf[:0]

	// metric itself
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
		Hash:   1,
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
		Hash:  1,
		Value: 111.222,
	}})
	if !assert.Equal(t, pbuf, buf.Bytes()) {
		t.Logf("Metric2:\n%vexp:\n%v", hex.Dump(buf.Bytes()), hex.Dump(pbuf))
	}
}

func TestHelperWriters(t *testing.T) {
	now = func() time.Time { return time.Unix(0, 0) }

	var w collectWriter

	l := New(
		NewFallbackWriter(
			NewLockedWriter(&errorWriter{err: errors.New("error")}),
			NewTeeWriter(&w, Discard),
		),
	)
	l.NoCaller = true
	l.NewID = func() ID { return ID{1, 2, 3, 4, 5} }

	l.RegisterMetric("name", MCounter, "", nil)
	l.Observe("name", 1, nil)
	l.SetLabels(Labels{"a", "b"})
	tr := l.Start()
	tr.Printf("message: %v", 2)
	tr.Finish()

	exp := []cev{
		{Ev: Meta{Type: MetaMetricDescription, Data: Labels{"name=name", "type=" + MCounter, "help=", "labels"}}},
		{Ev: Metric{Name: "name", Value: 1}},
		{Ev: Labels{"a", "b"}},
		{Ev: SpanStart{ID: tr.ID}},
		{ID: tr.ID, Ev: Message{Text: "message: 2"}},
		{Ev: SpanFinish{ID: tr.ID}},
	}
	assert.Equal(t, exp, w.Events)

	w.Events = w.Events[:0]
	var fb collectWriter

	l.Writer = NewFallbackWriter(
		&w,
		&fb,
	)

	l.RegisterMetric("name", MCounter, "", nil)
	l.Observe("name", 1, nil)
	l.SetLabels(Labels{"a", "b"})
	tr = l.Start()
	tr.Printf("message: %v", 2)
	tr.Finish()

	assert.Equal(t, exp, w.Events)
}

func TestMetricsCache(t *testing.T) {
	var b, exp bufWriter

	w := NewJSONWriter(&b)

	l := New(w)

	l.Observe("name", 1, Labels{"a=b"})
	assert.Equal(t, `{"v":{"h":1,"v":1,"n":"name","L":["a=b"]}}`+"\n", string(b))

	b = b[:0]

	l.Observe("name", 1, Labels{"a=c"})
	assert.Equal(t, `{"v":{"h":2,"v":1,"n":"name","L":["a=c"]}}`+"\n", string(b))

	b = b[:0]

	for i := 0; i < metricsCacheMaxValues+10; i++ {
		l.Observe("name", float64(i), Labels{"a=b"})

		exp = AppendPrintf(exp[:0], `{"v":{"h":%d,"v":%d}}`+"\n", 1, i)
		assert.Equal(t, exp, b)

		b = b[:0]

		l.Observe("name", float64(i), Labels{"a=c"})

		exp = AppendPrintf(exp[:0], `{"v":{"h":%d,"v":%d}}`+"\n", 2, i)
		assert.Equal(t, exp, b)

		b = b[:0]
	}
}

func TestMetricsCacheKill(t *testing.T) {
	metricsCacheMaxValues = 10

	var b, exp bufWriter

	w := NewJSONWriter(&b)

	l := New(w)

	for i := 0; i < metricsCacheMaxValues+1; i++ {
		l.Observe("name", float64(i), Labels{fmt.Sprintf("label=%d", i)})

		exp = AppendPrintf(exp[:0], `{"v":{"h":%d,"v":%d,"n":"name","L":["label=%d"]}}`+"\n", i+1, i, i)
		assert.Equal(t, exp, b)

		b = b[:0]

		if t.Failed() {
			return
		}
	}

	for i := 0; i < 3; i++ {
		l.Observe("name", float64(i), Labels{fmt.Sprintf("label=%d", i)})

		exp = AppendPrintf(exp[:0], `{"v":{"v":%d,"n":"name","L":["label=%d"]}}`+"\n", i, i)
		assert.Equal(t, exp, b)

		b = b[:0]

		if t.Failed() {
			t.Logf("cache (%d): %v", len(w.cc), w.cc)
			return
		}
	}
}

func TestMetricsCacheKill2(t *testing.T) {
	metricsCacheMaxValues = 100
	metricsTest = true
	defer func() {
		metricsTest = false
	}()

	var b, exp bufWriter

	w := NewJSONWriter(&b)

	l := New(w)

	for i := 0; i < metricsCacheMaxValues; i++ {
		l.Observe("name", float64(i), Labels{fmt.Sprintf("label=%d", i)})

		exp = AppendPrintf(exp[:0], `{"v":{"h":%d,"v":%d,"n":"name","L":["label=%d"]}}`+"\n", i+1, i, i)
		assert.Equal(t, exp, b)

		b = b[:0]

		if t.Failed() {
			return
		}
	}

	for i := 0; i < metricsCacheMaxValues; i++ {
		l.Observe("name2", float64(i), Labels{fmt.Sprintf("label=%d", i)})

		exp = AppendPrintf(exp[:0], `{"v":{"h":%d,"v":%d,"n":"name2","L":["label=%d"]}}`+"\n", metricsCacheMaxValues+i+1, i, i)
		assert.Equal(t, exp, b)

		b = b[:0]

		if t.Failed() {
			return
		}
	}

	l.Observe("name3", float64(0), Labels{"a=b", "c=d"})

	//	t.Logf("cache before: %d | %v %v", len(w.cc), w.skip, w.cc)

	l.Observe("name", float64(0), Labels{fmt.Sprintf("label=%d", metricsCacheMaxValues)})

	//	t.Logf("cache after:  %d | %v %v", len(w.cc), w.skip, w.cc)

	b = b[:0]

	for i := 0; i < 3; i++ {
		l.Observe("name", float64(i), Labels{fmt.Sprintf("label=%d", i)})

		exp = AppendPrintf(exp[:0], `{"v":{"v":%d,"n":"name","L":["label=%d"]}}`+"\n", i, i)
		assert.Equal(t, exp, b)

		b = b[:0]

		if t.Failed() {
			t.Logf("cache (%d): %v", len(w.cc), w.cc)
			return
		}
	}

	for i := 0; i < 3; i++ {
		l.Observe("name2", float64(i), Labels{fmt.Sprintf("label=%d", i)})

		exp = AppendPrintf(exp[:0], `{"v":{"h":%d,"v":%d}}`+"\n", metricsCacheMaxValues+i+1, i)
		assert.Equal(t, exp, b)

		b = b[:0]

		if t.Failed() {
			t.Logf("cache (%d): %v", len(w.cc), w.cc)
			return
		}
	}

	l.Observe("name3", float64(1.1), Labels{"a=b", "c=d"})

	exp = AppendPrintf(exp[:0], `{"v":{"h":%d,"v":%v}}`+"\n", 2*metricsCacheMaxValues+1, 1.1)
	assert.Equal(t, exp, b)

	b = b[:0]
}

func TestTeeWriter(t *testing.T) {
	var buf1, buf2 bytes.Buffer

	w1 := NewJSONWriter(&buf1)
	w2 := NewJSONWriter(&buf2)

	w := NewTeeWriter(w1, w2)

	fe := Funcentry(-1)
	loc := Caller(-1)

	_ = w.Labels(Labels{"a=b", "f"}, ID{})
	_ = w.Message(Message{PC: loc, Text: "msg", Time: 1}, ID{})
	_ = w.SpanStarted(SpanStart{ID{100}, ID{}, time.Date(2019, 7, 6, 10, 18, 32, 0, time.UTC).UnixNano(), fe})
	_ = w.SpanFinished(SpanFinish{ID{100}, time.Second.Nanoseconds()})

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

func TestConsoleMetrics(t *testing.T) {
	var buf bytes.Buffer

	w := NewConsoleWriter(&buf, 0)
	l := New(NewTeeWriter(w))

	l.RegisterMetric("name", MCounter, "help 1", Labels{"labels1"})
	l.Observe("name", 3, Labels{"labels=again"})

	assert.Equal(t, `Meta: metric_desc ["name=name" "type=counter" "help=help 1" "labels" "labels1"]
name          3.00000  labels=again
`, buf.String())
}

//nolint:lll
func TestAttributes(t *testing.T) {
	now = func() time.Time { return time.Unix(0, 0) }

	var js, pb bytes.Buffer

	l := New(NewJSONWriter(&js), NewProtoWriter(&pb))
	l.NoCaller = true

	id := ID{1, 2, 3, 4, 5, 6}

	l.Printw("helpers",
		AInt("int", -1),
		AInt64("int64", -2),
		AUint64("uint64", 3),
		AFloat("float64", 4.5),
		AString("str", "string"),
		AID("id", id),
		AError("err", errors.New("some_err")),
	)

	l.Printw("slice", Attrs{
		{Name: "int32", Value: int32(-1)},
		{Name: "int16", Value: int16(-2)},
		{Name: "int8", Value: int8(-3)},
		{Name: "uint", Value: uint(4)},
		{Name: "uint32", Value: uint32(5)},
		{Name: "uint16", Value: uint16(6)},
		{Name: "uint8", Value: uint8(7)},
		{Name: "float32", Value: float32(8.5)},
	}...)

	//	l.Printw("nil", Attr{Name: "nil", Value: nil})

	l.Printw("undef", Attr{Name: "undef", Value: &bwr{}})

	assert.Equal(t, `{"m":{"m":"helpers","a":[{"n":"int","t":"i","v":-1},{"n":"int64","t":"i","v":-2},{"n":"uint64","t":"u","v":3},{"n":"float64","t":"f","v":4.5},{"n":"str","t":"s","v":"string"},{"n":"id","t":"d","v":"01020304050600000000000000000000"},{"n":"err","t":"s","v":"some_err"}]}}
{"m":{"m":"slice","a":[{"n":"int32","t":"i","v":-1},{"n":"int16","t":"i","v":-2},{"n":"int8","t":"i","v":-3},{"n":"uint","t":"u","v":4},{"n":"uint32","t":"u","v":5},{"n":"uint16","t":"u","v":6},{"n":"uint8","t":"u","v":7},{"n":"float32","t":"f","v":8.5}]}}
{"m":{"m":"undef","a":[{"n":"undef","t":"?","v":"*tlog.bwr"}]}}
`, js.String())

	var pbuf []byte

	pbuf = encode(pbuf[:0], &tlogpb.Record{Message: &tlogpb.Message{
		Text: "helpers",
		Attrs: []*tlogpb.Attr{
			{Name: "int", Type: 'i', Int: -1},
			{Name: "int64", Type: 'i', Int: -2},
			{Name: "uint64", Type: 'u', Uint: 3},
			{Name: "float64", Type: 'f', Float: 4.5},
			{Name: "str", Type: 's', Str: "string"},
			{Name: "id", Type: 'd', Bytes: id[:]},
			{Name: "err", Type: 's', Str: "some_err"},
		},
	}})

	i := len(pbuf)
	if !assert.Equal(t, pbuf, pb.Bytes()[:i]) {
		t.Logf("helpers:\n%v", hex.Dump(pbuf))
	}

	pbuf = encode(pbuf[:0], &tlogpb.Record{Message: &tlogpb.Message{
		Text: "slice",
		Attrs: []*tlogpb.Attr{
			{Name: "int32", Type: 'i', Int: -1},
			{Name: "int16", Type: 'i', Int: -2},
			{Name: "int8", Type: 'i', Int: -3},
			{Name: "uint", Type: 'u', Uint: 4},
			{Name: "uint32", Type: 'u', Uint: 5},
			{Name: "uint16", Type: 'u', Uint: 6},
			{Name: "uint8", Type: 'u', Uint: 7},
			{Name: "float32", Type: 'f', Float: 8.5},
		},
	}})

	assert.Equal(t, pbuf, pb.Bytes()[i:i+len(pbuf)])
	i += len(pbuf)

	pbuf = encode(pbuf[:0], &tlogpb.Record{Message: &tlogpb.Message{
		Text: "undef",
		Attrs: []*tlogpb.Attr{
			{Name: "undef", Type: '?', Str: "*tlog.bwr"},
		},
	}})

	assert.Equal(t, pbuf, pb.Bytes()[i:])
}

//nolint:gocognit
func BenchmarkWriter(b *testing.B) {
	loc := Caller(0)

	for _, ws := range []struct {
		name string
		nw   func(w io.Writer) Writer
	}{
		{"ConsoleStd", func(w io.Writer) Writer {
			return NewConsoleWriter(w, LstdFlags)
		}},
		{"ConsoleDet", func(w io.Writer) Writer {
			return NewConsoleWriter(w, LdetFlags)
		}},
		{"JSON", func(w io.Writer) Writer {
			return NewJSONWriter(w)
		}},
		{"Proto", func(w io.Writer) Writer {
			return NewProtoWriter(w)
		}},
	} {
		ws := ws

		b.Run(ws.name, func(b *testing.B) {
			for _, par := range []bool{false, true} {
				par := par

				n := SingleThread
				if par {
					n = Parallel
				}

				b.Run(n, func(b *testing.B) {
					ls := Labels{"a=b", "c=d"}
					msg := "some message"

					var cw CountableIODiscard
					w := ws.nw(&cw)

					for _, tc := range []struct {
						name string
						act  func(i int)
					}{
						{"TracedMessage", func(i int) {
							_ = w.Message(Message{
								PC:   loc,
								Time: 1,
								Text: msg,
							}, ID{1, 2, 3, 4, 5, 6, 7, 8})
						}},
						{"TracedMessage_NoLoc", func(i int) {
							_ = w.Message(Message{
								Time: 1,
								Text: msg,
							}, ID{1, 2, 3, 4, 5, 6, 7, 8})
						}},
						{"TracedMetric", func(i int) {
							_ = w.Metric(Metric{
								Name:   "some_fully_qualified_metric",
								Value:  123.456,
								Labels: ls,
							}, ID{1, 2, 3, 4, 5, 6, 7, 8})
						}},
					} {
						tc := tc

						b.Run(tc.name, func(b *testing.B) {
							b.ReportAllocs()
							cw.N, cw.B = 0, 0

							if par {
								b.RunParallel(func(b *testing.PB) {
									i := 0
									for b.Next() {
										i++
										tc.act(i)
									}
								})
							} else {
								for i := 0; i < b.N; i++ {
									tc.act(i)
								}
							}

							cw.ReportDisk(b)
						})
					}
				})
			}
		})
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
