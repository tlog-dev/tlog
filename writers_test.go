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

	var e Encoder

	w := NewReWriter(func(have io.Writer, err error) (io.Writer, error) {
		var b low.Buf

		files = append(files, &b)

		ewr.Writer = &b

		return &ewr, nil
	})

	e.Writer = w

	err := e.Encode(nil, []interface{}{"key", "value"})
	assert.NoError(t, err)

	ewr.nerr++

	err = e.Encode(nil, []interface{}{"key2", "value2"})
	assert.NoError(t, err)

	err = e.Encode(nil, []interface{}{KeyLabels, Labels{"label"}})
	assert.NoError(t, err)

	ewr.nerr++

	err = e.Encode(nil, []interface{}{"key3", "value3"})
	assert.NoError(t, err)

	ewr.nerr++

	err = e.Encode(nil, []interface{}{KeyLabels, Labels{"label2"}})
	assert.NoError(t, err)

	err = e.Encode(nil, []interface{}{"key4", "value4"})
	assert.NoError(t, err)

	ewr.nerr++
	ewr.nerr++

	err = e.Encode(nil, []interface{}{KeyLabels, Labels{"label3"}})
	assert.Error(t, err, ewr.err.Error())

	err = e.Encode(nil, []interface{}{"key5", "value5"})
	assert.NoError(t, err)

	ewr.nerr++
	ewr.nerr++

	err = e.Encode(nil, []interface{}{"key6", "value6"})
	assert.Error(t, err, ewr.err.Error())

	ewr.nerr++

	err = e.Encode(nil, []interface{}{"key7", "value7"})
	assert.NoError(t, err)

	exp := []*low.Buf{
		newfile([][]interface{}{
			{"key", "value"},
		}),
		newfile([][]interface{}{
			{"key2", "value2"},
			{KeyLabels, Labels{"label"}},
		}),
		newfile([][]interface{}{
			{KeyLabels, Labels{"label"}},
			{"key3", "value3"},
		}),
		newfile([][]interface{}{
			{KeyLabels, Labels{"label2"}},
			{"key4", "value4"},
		}),
		newfile([][]interface{}{
			{KeyLabels, Labels{"label3"}},
			{"key5", "value5"},
		}),
		newfile([][]interface{}{}),
		newfile([][]interface{}{
			{KeyLabels, Labels{"label3"}},
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

	e := Encoder{Writer: b}

	for _, evs := range events {
		e.Encode(nil, evs)
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
