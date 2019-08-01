package tlog

import (
	"bytes"
	"encoding/hex"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"

	"github.com/nikandfor/tlog/tlogpb"
)

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

	w.Message(
		Message{
			Location: loc,
			Time:     time.Duration(2 * 1000),
			Format:   "%v",
			Args:     []interface{}{4},
		},
		Span{ID: 10},
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
		Span:     10,
		Location: int64(loc),
		Time:     2,
		Text:     "4",
	}})
	assert.Equal(t, pbuf.Bytes()[l:], buf.Bytes()[l:])
	t.Logf("Message:\n%vexp:\n%v", hex.Dump(buf.Bytes()[l:]), hex.Dump(pbuf.Bytes()[l:]))

	buf.Reset()
	pbuf.Reset()
	delete(w.ls, loc)

	w.SpanStarted(
		Span{
			ID:      10,
			Started: time.Unix(0, 2*1000),
		},
		3,
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
		Id:       10,
		Parent:   3,
		Location: int64(loc),
		Started:  2,
	}})
	assert.Equal(t, pbuf.Bytes()[l:], buf.Bytes()[l:])
	t.Logf("SpanStart:\n%vexp:\n%v", hex.Dump(buf.Bytes()[l:]), hex.Dump(pbuf.Bytes()[l:]))

	buf.Reset()
	pbuf.Reset()

	w.SpanFinished(
		Span{
			ID:    10,
			Flags: 1000,
		},
		time.Second,
	)
	_ = pbuf.EncodeMessage(&tlogpb.Record{SpanFinish: &tlogpb.SpanFinish{
		Id:      10,
		Elapsed: time.Second.Nanoseconds() / 1000,
		Flags:   1000,
	}})
	assert.Equal(t, pbuf.Bytes(), buf.Bytes())
	t.Logf("SpanFinish:\n%vexp:\n%v", hex.Dump(buf.Bytes()), hex.Dump(pbuf.Bytes()))

	buf.Reset()
	pbuf.Reset()
}
