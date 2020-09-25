// +build linux darwin

package tlogdb

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/nikandfor/xrain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/parse"
)

var tlogV = flag.String("tlog-v", "", "")

func TestDBSmoke(t *testing.T) {
	tl = tlog.NewTestLogger(t, *tlogV, nil)
	tlog.DefaultLogger = tl

	const N = 3

	b, err := xrain.Mmap("/tmp/tlogdb.xrain", os.O_CREATE|os.O_RDWR|os.O_TRUNC)
	require.NoError(t, err)

	l := xrain.NewKVLayout2(nil)
	l.Compare = func(a, b []byte) int {
		return bytes.Compare(b, a)
	}

	db, err := xrain.NewDB(b, 0x200, nil)
	require.NoError(t, err)

	d := NewDB(db)

	w, err := NewWriter(d)
	require.NoError(t, err)

	tm := int64(0)
	now := func() int64 {
		tm++
		return tm
	}

	for i := 0; i < N; i++ {
		err = w.Message(parse.Message{
			Time: now(),
			Text: fmt.Sprintf("message: %d.%d", i, 1),
		})

		assert.NoError(t, err)
	}

	err = db.View(func(tx *xrain.Tx) error {
		xrain.DebugDump(tl.IOWriter(0), tx.SimpleBucket)

		return nil
	})
	require.NoError(t, err)

	var msgs []*parse.Message
	var next []byte

	tl.Printf("=== scan with no query ===")

	for lim := 10; lim >= 0; lim-- {
		var e []*parse.Message
		e, next, err = d.Messages(nil, ID{}, "", next, 2)
		if !assert.NoError(t, err) {
			break
		}

		msgs = append(msgs, e...)

		if next == nil {
			break
		}
	}

	if assert.Len(t, msgs, N) {
		assert.Equal(t, []*parse.Message{
			{
				Time: 1,
				Text: "message: 0.1",
			}, {
				Time: 2,
				Text: "message: 1.1",
			}, {
				Time: 3,
				Text: "message: 2.1",
			},
		}, msgs)
	}

	msgs = msgs[:0]

	tl.Printf("=== scan with query 'essa' ===")

	for lim := 10; lim >= 0; lim-- {
		var e []*parse.Message
		e, next, err = d.Messages(nil, ID{}, "essa", next, 2)
		if !assert.NoError(t, err) {
			break
		}

		msgs = append(msgs, e...)

		if next == nil {
			break
		}
	}

	if assert.Len(t, msgs, N) {
		assert.Equal(t, []*parse.Message{
			{
				Time: 1,
				Text: "message: 0.1",
			}, {
				Time: 2,
				Text: "message: 1.1",
			}, {
				Time: 3,
				Text: "message: 2.1",
			},
		}, msgs)
	}

	msgs = msgs[:0]

	tl.Printf("=== scan with query '1.1' ===")

	for lim := 10; lim >= 0; lim-- {
		var e []*parse.Message
		e, next, err = d.Messages(nil, ID{}, "1.1", next, 2)
		if !assert.NoError(t, err) {
			break
		}

		msgs = append(msgs, e...)

		if next == nil {
			break
		}
	}

	if assert.Len(t, msgs, 1) {
		assert.Equal(t, []*parse.Message{
			{
				Time: 2,
				Text: "message: 1.1",
			},
		}, msgs)
	}
}
