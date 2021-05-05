package tlog

import (
	"io"
	"testing"

	"github.com/nikandfor/errors"
	"github.com/stretchr/testify/assert"

	"github.com/nikandfor/tlog/low"
)

type errWriter struct {
	io.Writer
	nerr int
	err  error
}

func TestReWriter(t *testing.T) {
	var files []*low.Buf

	ewr := errWriter{
		err: errors.New("some"),
	}

	var b low.Buf
	var e Encoder

	w := NewReWriter(func(have io.Writer, err error) (io.Writer, error) {
		var b low.Buf

		files = append(files, &b)

		ewr.Writer = &b

		return &ewr, nil
	})

	encode := func(kvs []interface{}) (err error) {
		b = b[:0]

		b = e.AppendTag(b, Map, e.CalcMapLen(kvs))
		b = e.AppendKVs(b, kvs)

		_, err = w.Write(b)

		return
	}

	err := encode([]interface{}{"key", "value"})
	assert.NoError(t, err)

	ewr.nerr++

	err = encode([]interface{}{"key2", "value2"})
	assert.NoError(t, err)

	err = encode([]interface{}{KeyEventType, EventLabels, KeyLabels, Labels{"label"}})
	assert.NoError(t, err)

	ewr.nerr++

	err = encode([]interface{}{"key3", "value3"})
	assert.NoError(t, err)

	ewr.nerr++

	err = encode([]interface{}{KeyEventType, EventLabels, KeyLabels, Labels{"label2"}})
	assert.NoError(t, err)

	err = encode([]interface{}{"key4", "value4"})
	assert.NoError(t, err)

	ewr.nerr++
	ewr.nerr++

	err = encode([]interface{}{KeyEventType, EventLabels, KeyLabels, Labels{"label3"}})
	assert.Error(t, err, ewr.err.Error())

	err = encode([]interface{}{"key5", "value5"})
	assert.NoError(t, err)

	ewr.nerr++
	ewr.nerr++

	err = encode([]interface{}{"key6", "value6"})
	assert.Error(t, err, ewr.err.Error())

	ewr.nerr++

	err = encode([]interface{}{"key7", "value7"})
	assert.NoError(t, err)

	exp := []*low.Buf{
		newfile([][]interface{}{
			{"key", "value"},
		}),
		newfile([][]interface{}{
			{"key2", "value2"},
			{KeyEventType, EventLabels, KeyLabels, Labels{"label"}},
		}),
		newfile([][]interface{}{
			{KeyEventType, EventLabels, KeyLabels, Labels{"label"}},
			{"key3", "value3"},
		}),
		newfile([][]interface{}{
			{KeyEventType, EventLabels, KeyLabels, Labels{"label2"}},
			{"key4", "value4"},
		}),
		newfile([][]interface{}{
			{KeyEventType, EventLabels, KeyLabels, Labels{"label3"}},
			{"key5", "value5"},
		}),
		newfile([][]interface{}{}),
		newfile([][]interface{}{
			{KeyEventType, EventLabels, KeyLabels, Labels{"label3"}},
			{"key7", "value7"},
		}),
	}

	assert.Equal(t, exp, files)

	if t.Failed() {
		for i, f := range files {
			t.Logf("dump %d:\n%s", i, Dump(*f))
		}
	}
}

func newfile(events [][]interface{}) (b *low.Buf) {
	b = new(low.Buf)

	var e Encoder

	for _, evs := range events {
		*b = e.AppendTag(*b, Map, e.CalcMapLen(evs))
		*b = e.AppendKVs(*b, evs)
	}

	return b
}

func (w *errWriter) Write(p []byte) (n int, err error) {
	if w.nerr != 0 && w.err != nil {
		err = w.err
		w.nerr--

		return
	}

	return w.Writer.Write(p)
}
