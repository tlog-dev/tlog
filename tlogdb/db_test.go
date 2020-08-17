package tlogdb

import (
	"fmt"
	"testing"

	"github.com/nikandfor/xrain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/parse"
)

func TestDBSmoke(t *testing.T) {
	tl = tlog.NewTestLogger(t, "", false)

	const N = 3

	b := xrain.NewMemBack(0)
	db, err := xrain.NewDB(b, 0, nil)
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

	var evs []Event
	var next []byte

	for lim := 10; lim >= 0; lim-- {
		var e []Event
		e, next, err = d.All(next, 2)
		if !assert.NoError(t, err) {
			break
		}

		evs = append(evs, e...)

		if next == nil {
			break
		}
	}

	if assert.Len(t, evs, N) {
		assert.Equal(t, []Event{
			{Message: &parse.Message{
				Time: 1,
				Text: "message: 0.1",
			}},
			{Message: &parse.Message{
				Time: 2,
				Text: "message: 1.1",
			}},
			{Message: &parse.Message{
				Time: 3,
				Text: "message: 2.1",
			}},
		}, evs)
	}
}
