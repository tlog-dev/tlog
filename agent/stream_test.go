package agent

import (
	"testing"
	"time"

	"github.com/nikandfor/assert"
	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/low"
	"github.com/nikandfor/tlog/wire"
)

func TestStream(t *testing.T) {
	fs := NewMemFS()
	fs.log = tlog.NewTestLogger(t, "", nil)

	ts := time.Now().Truncate(time.Second)

	s, err := NewStream(fs)
	assert.NoError(t, err)

	// empty reader

	r, err := s.OpenReader()
	assert.NoError(t, err)

	data := make(low.Buf, 0, 100)

	m, err := r.WriteTo(&data)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), m)

	// write

	msg := encode(nil, tlog.KeyTime, ts, "message", "message_text")
	ts = ts.Add(time.Second)

	n, err := s.Write(msg)
	assert.NoError(t, err)
	assert.Equal(t, len(msg), n)

	// read now

	m, err = r.WriteTo(&data)
	assert.NoError(t, err)
	assert.Equal(t, int64(len(msg)), m)

	assert.Equal(t, msg, []byte(data[:m]))

	// close

	err = r.Close()
	assert.NoError(t, err)

	err = s.Close()
	assert.NoError(t, err)

	// reopen

	s, err = NewStream(fs)
	assert.NoError(t, err)

	r, err = s.OpenReader()
	assert.NoError(t, err)

	// reread

	m, err = r.WriteTo(&data)
	assert.NoError(t, err)
	assert.Equal(t, int64(len(msg)), m)

	assert.Equal(t, msg, []byte(data[:m]))
}

func encode(buf []byte, kvs ...interface{}) []byte {
	var e wire.Encoder

	buf = e.AppendMap(buf, -1)

	buf = tlog.AppendKVs(&e, buf, kvs)

	buf = e.AppendBreak(buf)

	return buf
}
