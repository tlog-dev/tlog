package tlbolt

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/low"
)

func TestSmoke(t *testing.T) {
	tmpf, err := os.CreateTemp("", "tlog_bbolt_")
	require.NoError(t, err)

	defer os.Remove(tmpf.Name())

	bdb, err := bbolt.Open(tmpf.Name(), 0744, nil)
	require.NoError(t, err)

	defer bdb.Close()

	tl = tlog.NewTestLogger(t, "", nil)

	db := NewWriter(bdb)

	l := tlog.New(db)
	l.NoCaller = true

	l.SetLabels(tlog.Labels{"a", "c=d"})

	l.Printw("first", "arg", "value", "arg2", 4)
	l.Printw("second", "arg", "qulue", "arg2", 4)
	l.Printw("third", "arg", "value", "arg2", 9)
	l.Printw("fourth", "arg", 1, "arg2", []byte("bytes"))

	tl.Printw("done")

	var buf low.Buf
	err = db.Dump(&buf)
	require.NoError(t, err)

	tl.Printf("dump\n%s", buf)

	evs, next, err := db.Events("", 10, nil, nil)
	require.NoError(t, err)
	assert.Nil(t, next)
	assert.Equal(t, 5, len(evs), "number of elements")

	fmt.Fprintf(tlog.Stderr, "dump events (%d)\n", len(evs))

	d := tlog.NewDumper(tlog.Stderr)

	for _, ev := range evs {
		_, _ = d.Write(ev)
	}
}
