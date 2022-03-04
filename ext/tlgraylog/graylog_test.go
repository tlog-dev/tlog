package tlgraylog

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/low"
	"github.com/nikandfor/tlog/wire"
)

func TestGraylog(t *testing.T) {
	ts := time.Date(2022, time.January, 15, 0, 18, 12, 0, time.Local)

	var c, b low.Buf
	var buf bytes.Buffer

	w := New(&c)
	w.Hostname = "myhost"

	b = encode(b[:0], &ts, "a", "b", "c", 4)
	_, err := w.Write(b)
	assert.NoError(t, err)
	assert.Equal(t, `{"version":"1.1","host":"myhost","timestamp":1642195092.000,"_a":"b","_c":4,"short_message":"_no_message_","level":6}`+"\n", string(c))

	err = json.Indent(&buf, c, "", "")
	assert.NoError(t, err)

	c = c[:0]

	b = encode(b[:0], &ts, "arr", []int{1, 2, 3}, "obj", encobj("one", 1, "two", 2), tlog.KeyLogLevel, tlog.Error)
	_, err = w.Write(b)
	assert.NoError(t, err)
	assert.Equal(t, `{"version":"1.1","host":"myhost","timestamp":1642195092.300,"_arr":"[1,2,3]","_obj":"{one:1,two:2}","short_message":"_no_message_","level":3}`+"\n", string(c))

	err = json.Indent(&buf, c, "", "")
	assert.NoError(t, err)

	c = c[:0]

	b = encode(b[:0], &ts, tlog.KeyLabels, tlog.Labels{"a=b", "c", "ddd=eee"})
	_, err = w.Write(b)
	assert.NoError(t, err)
	assert.Equal(t, `{"version":"1.1","host":"myhost","timestamp":1642195092.600,"_L_a":"b","_L_c":"","_L_ddd":"eee","short_message":"_no_message_","level":6}`+"\n", string(c))

	err = json.Indent(&buf, c, "", "")
	assert.NoError(t, err)

	c = c[:0]
}

func encode(b []byte, ts *time.Time, kvs ...interface{}) []byte {
	var e wire.Encoder

	b = e.AppendMap(b, -1)

	b = e.AppendKey(b, tlog.KeyTime)
	b = e.AppendTime(b, *ts)
	*ts = ts.Add(300 * time.Millisecond)

	b = tlog.AppendKVs(&e, b, kvs)

	b = e.AppendBreak(b)

	return b
}

func encobj(kvs ...interface{}) tlog.RawMessage {
	var e wire.Encoder

	var b []byte

	b = e.AppendMap(b, -1)
	b = tlog.AppendKVs(&e, b, kvs)
	b = e.AppendBreak(b)

	return b
}
