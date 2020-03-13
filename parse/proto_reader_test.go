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

func testReader(t *testing.T, neww func(io.Writer) tlog.Writer, newr func(io.Reader) Reader) {
	const Prefix = "github.com/nikandfor/tlog/"

	var buf bytes.Buffer
	tm := time.Date(2019, 7, 31, 18, 21, 2, 0, time.UTC)
	now := func() time.Time {
		tm = tm.Add(time.Second)
		return tm
	}

	w := neww(&buf)

	w.Labels(Labels{"a", "b=c"})

	w.Message(tlog.Message{
		Location: tlog.Caller(0),
		Time:     time.Duration(now().UnixNano()),
		Format:   "%v",
		Args:     []interface{}{3},
	}, tlog.Span{})

	w.SpanStarted(tlog.Span{
		ID:      ID{1},
		Started: now(),
	}, tlog.ZeroID, tlog.Caller(0))

	w.Message(tlog.Message{
		Location: tlog.Caller(0),
		Time:     time.Duration(now().UnixNano()),
		Format:   "%v",
		Args:     []interface{}{5},
	}, tlog.Span{ID: ID{1}})

	w.SpanStarted(tlog.Span{
		ID:      ID{2},
		Started: now(),
	}, ID{1}, tlog.Caller(0))

	w.SpanFinished(tlog.Span{ID: ID{2}}, time.Second)

	w.SpanFinished(tlog.Span{ID: ID{1}}, 2*time.Second)

	t.Logf("data:\n%v", hex.Dump(buf.Bytes()))

	// read
	r := newr(&buf)
	if r, ok := r.(*ProtoReader); ok {
		r.buf = r.buf[:10]
	}

	var res []interface{}
	var err error
	locs := map[uintptr]uintptr{}

	for {
		var o interface{}

		o, err = r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Errorf("Read: %v", err)
			break
		}

		switch t := o.(type) {
		case Location:
			ex, ok := locs[t.PC]
			if !ok {
				ex = uintptr(len(locs) + 1)
				locs[t.PC] = ex
			}
			t.PC = ex
			t.File = strings.TrimPrefix(t.File, Prefix) // cut prefix in case of repo is not in GOPATH or similar folder structure
			o = t
		case Message:
			t.Location = locs[t.Location]
			o = t
		case SpanStart:
			t.Location = locs[t.Location]
			t.Started = t.Started.UTC()
			o = t
		}

		res = append(res, o)
	}

	if err != io.EOF {
		return
	}

	tm = time.Date(2019, 7, 31, 18, 21, 2, 0, time.UTC)

	assert.Equal(t, []interface{}{
		Labels{"a", "b=c"},
		Location{
			PC:   1,
			Name: "github.com/nikandfor/tlog/parse.testReader",
			File: "parse/proto_reader_test.go",
			Line: 31,
		},
		Message{
			Span:     ID{},
			Location: 1,
			Time:     time.Duration(now().UnixNano()),
			Text:     "3",
		},
		Location{
			PC:   2,
			Name: "github.com/nikandfor/tlog/parse.testReader",
			File: "parse/proto_reader_test.go",
			Line: 40,
		},
		SpanStart{
			ID:       ID{1},
			Parent:   ID{},
			Location: 2,
			Started:  now(),
		},
		Location{
			PC:   3,
			Name: "github.com/nikandfor/tlog/parse.testReader",
			File: "parse/proto_reader_test.go",
			Line: 43,
		},
		Message{
			Span:     ID{1},
			Location: 3,
			Time:     time.Duration(now().UnixNano()),
			Text:     "5",
		},
		Location{
			PC:   4,
			Name: "github.com/nikandfor/tlog/parse.testReader",
			File: "parse/proto_reader_test.go",
			Line: 52,
		},
		SpanStart{
			ID:       ID{2},
			Parent:   ID{1},
			Location: 4,
			Started:  now(),
		},
		SpanFinish{
			ID:      ID{2},
			Elapsed: 1 * time.Second,
		},
		SpanFinish{
			ID:      ID{1},
			Elapsed: 2 * time.Second,
		},
	}, res)
}

func TestProtoReader(t *testing.T) {
	testReader(t,
		func(w io.Writer) tlog.Writer { return tlog.NewProtoWriter(w) },
		func(r io.Reader) Reader { return NewProtoReader(r) },
	)
}
