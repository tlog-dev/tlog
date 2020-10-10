package parse

import (
	"bytes"
	"encoding/hex"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/nikandfor/tlog"
)

func testReader(t *testing.T, neww func(io.Writer) tlog.Writer, newr func(io.Reader, *tlog.Logger) Reader) {
	const Prefix = "github.com/nikandfor/tlog/"

	tl := tlog.NewTestLogger(t, "", tlog.Stderr)

	var buf bytes.Buffer
	tm := time.Date(2019, 7, 31, 18, 21, 2, 0, time.Local)
	now := func() int64 {
		tm = tm.Add(time.Second)
		return tm.UnixNano()
	}

	w := neww(&buf)

	_ = w.Labels(tlog.Labels{"a", "b=c"}, ID{1, 2, 3, 4})

	_ = w.Meta(tlog.Meta{
		Type: "metric_desc",
		Data: tlog.Labels{
			"name=op_metric",
			"type=type",
			"help=help message",
			"labels",
			"mode=debug",
		},
	})

	_ = w.Message(tlog.Message{
		PC:   tlog.Caller(0),
		Time: now(),
		Text: "3",
	}, tlog.ID{})

	_ = w.SpanStarted(tlog.SpanStart{
		ID:        ID{1},
		Parent:    ID{},
		StartedAt: now(),
		PC:        tlog.Caller(0),
	})

	_ = w.Message(tlog.Message{
		PC:   tlog.Caller(0),
		Time: now(),
		Text: "5",
	}, ID{1})

	_ = w.SpanStarted(tlog.SpanStart{
		ID:        ID{2},
		Parent:    ID{1},
		StartedAt: now(),
		PC:        tlog.Caller(0),
	})

	_ = w.Metric(
		tlog.Metric{
			Name:   "op_metric",
			Value:  123.456789,
			Labels: tlog.Labels{"path=/url/path", "algo=fast"},
		},
		ID{2},
	)

	_ = w.SpanFinished(tlog.SpanFinish{
		ID:      ID{2},
		Elapsed: time.Second.Nanoseconds(),
	})

	_ = w.SpanFinished(tlog.SpanFinish{
		ID:      ID{1},
		Elapsed: 2 * time.Second.Nanoseconds(),
	})

	_ = w.Message(tlog.Message{
		Level: tlog.LevelError,
		Text:  "attrs",
		Attrs: Attrs{
			{Name: "id", Value: ID{1, 2, 3, 4, 5}},
			{Name: "str", Value: "string"},
			{Name: "int", Value: int(-5)},
			{Name: "uint", Value: uint(5)},
			{Name: "float", Value: 1.12},
		},
	}, ID{})

	t.Logf("data:\n%v", hex.Dump(buf.Bytes()))

	// read
	r := newr(&buf, tl)
	if r, ok := r.(*ProtoReader); ok {
		r.buf = r.buf[:0:10]
	}

	var res []interface{}
	var err error
	locs := map[uint64]uint64{}

	for {
		var o interface{}

		o, err = r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Errorf("Read: %+v (%T)", err, err)
			break
		}

		switch t := o.(type) {
		case Frame:
			ex, ok := locs[t.PC]
			if !ok {
				ex = uint64(len(locs) + 1)
				locs[t.PC] = ex
			}
			t.PC = ex
			t.Entry = 0x100 + ex

			for _, p := range strings.Split(Prefix, "/") {
				t.File = strings.TrimPrefix(t.File, p+"/") // trim prefix in case of repo is not in GOPATH or similar folder structure
			}

			o = t
		case Message:
			t.PC = locs[t.PC]
			o = t
		case Metric:
			t.Hash = 0
			o = t
		case SpanStart:
			t.PC = locs[t.PC]
			o = t
		}

		res = append(res, o)
	}

	if err != io.EOF {
		return
	}

	tm = time.Date(2019, 7, 31, 18, 21, 2, 0, time.Local)

	assert.Equal(t, []interface{}{
		Labels{Labels: tlog.Labels{"a", "b=c"}, Span: ID{1, 2, 3, 4}},
		Meta{
			Type: "metric_desc",
			Data: tlog.Labels{
				"name=op_metric",
				"type=type",
				"help=help message",
				"labels",
				"mode=debug",
			},
		},
		Frame{
			PC:    1,
			Entry: 0x101,
			Name:  "github.com/nikandfor/tlog/parse.testReader",
			File:  "parse/proto_reader_test.go",
			Line:  44,
		},
		Message{
			Span: ID{},
			PC:   1,
			Time: now(),
			Text: "3",
		},
		Frame{
			PC:    2,
			Entry: 0x102,
			Name:  "github.com/nikandfor/tlog/parse.testReader",
			File:  "parse/proto_reader_test.go",
			Line:  53,
		},
		SpanStart{
			ID:        ID{1},
			Parent:    ID{},
			PC:        2,
			StartedAt: now(),
		},
		Frame{
			PC:    3,
			Entry: 0x103,
			Name:  "github.com/nikandfor/tlog/parse.testReader",
			File:  "parse/proto_reader_test.go",
			Line:  57,
		},
		Message{
			Span: ID{1},
			PC:   3,
			Time: now(),
			Text: "5",
		},
		Frame{
			PC:    4,
			Entry: 0x104,
			Name:  "github.com/nikandfor/tlog/parse.testReader",
			File:  "parse/proto_reader_test.go",
			Line:  66,
		},
		SpanStart{
			ID:        ID{2},
			Parent:    ID{1},
			PC:        4,
			StartedAt: now(),
		},
		Metric{
			Span:   ID{2},
			Labels: tlog.Labels{"path=/url/path", "algo=fast"},
			Name:   "op_metric",
			Value:  123.456789,
		},
		SpanFinish{
			ID:      ID{2},
			Elapsed: 1 * time.Second.Nanoseconds(),
		},
		SpanFinish{
			ID:      ID{1},
			Elapsed: 2 * time.Second.Nanoseconds(),
		},
		Message{
			Level: tlog.LevelError,
			Text:  "attrs",
			Attrs: Attrs{
				{Name: "id", Value: ID{1, 2, 3, 4, 5}},
				{Name: "str", Value: "string"},
				{Name: "int", Value: int64(-5)},
				{Name: "uint", Value: uint64(5)},
				{Name: "float", Value: 1.12},
			},
		},
	}, res)
}

func TestProtoReader(t *testing.T) {
	testReader(t,
		func(w io.Writer) tlog.Writer { return tlog.NewProtoWriter(w) },
		func(r io.Reader, tl *tlog.Logger) Reader { rd := NewProtoReader(r); rd.l = tl; return rd },
	)
}
