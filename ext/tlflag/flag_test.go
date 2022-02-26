package tlflag

import (
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/compress"
	"github.com/nikandfor/tlog/convert"
	"github.com/nikandfor/tlog/rotated"
	"github.com/nikandfor/tlog/tlio"
	"github.com/stretchr/testify/assert"
)

type testFile string

func TestFileExtWriter(t *testing.T) {
	OpenFileWriter = func(n string, f int, m os.FileMode) (io.Writer, error) {
		return testFile(n), nil
	}
	CompressorBlockSize = 1 * compress.KiB

	w, err := OpenWriter("stderr")
	assert.NoError(t, err)
	assert.Equal(t, tlio.TeeWriter{
		tlog.NewConsoleWriter(tlog.Stderr, tlog.LstdFlags),
	}, w)

	w, err = OpenWriter("stderr+dm")
	assert.NoError(t, err)
	assert.Equal(t, tlio.TeeWriter{
		tlog.NewConsoleWriter(tlog.Stderr, tlog.LdetFlags|tlog.Lmilliseconds),
	}, w)

	w, err = OpenWriter("stderr.json")
	assert.NoError(t, err)
	assert.Equal(t, tlio.TeeWriter{
		convert.NewJSONWriter(tlog.Stderr),
	}, w)

	w, err = OpenWriter("stderr.json+TU")
	assert.NoError(t, err)
	assert.Equal(t, tlio.TeeWriter{
		func() *convert.JSON {
			w := convert.NewJSONWriter(tlog.Stderr)

			w.TimeFormat = JSONDefaultTimeFormat
			w.TimeZone = time.UTC

			return w
		}(),
	}, w)

	w, err = OpenWriter("stderr.json+T(150405)LU")
	assert.NoError(t, err)
	assert.Equal(t, tlio.TeeWriter{
		func() *convert.JSON {
			w := convert.NewJSONWriter(tlog.Stderr)

			w.TimeFormat = "150405"
			w.TimeZone = time.UTC

			return w
		}(),
	}, w)

	w, err = OpenWriter("stderr+dm,stderr.json")
	assert.NoError(t, err)
	assert.Equal(t, tlio.TeeWriter{
		tlog.NewConsoleWriter(tlog.Stderr, tlog.LdetFlags|tlog.Lmilliseconds),
		convert.NewJSONWriter(tlog.Stderr),
	}, w)

	w, err = OpenWriter("stderr+dm,./stderr.json")
	assert.NoError(t, err)
	assert.Equal(t, tlio.TeeWriter{
		tlog.NewConsoleWriter(tlog.Stderr, tlog.LdetFlags|tlog.Lmilliseconds),
		tlio.WriteCloser{
			Writer: convert.NewJSONWriter(testFile("./stderr.json")),
			Closer: testFile("./stderr.json"),
		},
	}, w)

	w, err = OpenWriter(".tl,-.tl")
	assert.NoError(t, err)
	assert.Equal(t, tlio.TeeWriter{
		tlio.NopCloser{Writer: tlog.Stderr},
		tlio.NopCloser{Writer: tlog.Stdout},
	}, w)

	w, err = OpenWriter("file.json.ez")
	assert.NoError(t, err)
	assert.Equal(t, tlio.WriteCloser{
		Writer: convert.NewJSONWriter(compress.NewEncoder(testFile("file.json.ez"), CompressorBlockSize)),
		Closer: testFile("file.json.ez"),
	}, w)

	w, err = OpenWriter(".tlz")
	assert.NoError(t, err)
	assert.Equal(t, tlio.TeeWriter{
		compress.NewEncoder(tlog.Stderr, CompressorBlockSize),
	}, w)

	w, err = OpenWriter("file.tlz")
	assert.NoError(t, err)
	assert.Equal(t, tlio.WriteCloser{
		Writer: compress.NewEncoder(testFile("file.tlz"), CompressorBlockSize),
		Closer: testFile("file.tlz"),
	}, w)

	w, err = OpenWriter("file_@.tlog")
	assert.NoError(t, err)
	f, ok := w.(*rotated.File)
	if assert.True(t, ok, "expected *rotated.File") {
		q := rotated.Create("file_@.tlog")

		f.OpenFile = nil
		q.OpenFile = nil

		assert.Equal(t, q, w)
	}
}

func TestConsoleWidth(t *testing.T) {
	w, err := OpenWriter("stderr+s")
	assert.NoError(t, err)
	assert.Equal(t, tlio.TeeWriter{
		func() *tlog.ConsoleWriter {
			w := tlog.NewConsoleWriter(tlog.Stderr, tlog.LstdFlags)
			w.IDWidth = len(tlog.ID{})

			return w
		}(),
	}, w)

	w, err = OpenWriter("stderr+S")
	assert.NoError(t, err)
	assert.Equal(t, tlio.TeeWriter{
		func() *tlog.ConsoleWriter {
			w := tlog.NewConsoleWriter(tlog.Stderr, tlog.LstdFlags)
			w.IDWidth = 2 * len(tlog.ID{})

			return w
		}(),
	}, w)

	w, err = OpenWriter("stderr+s[10]")
	assert.NoError(t, err)
	assert.Equal(t, tlio.TeeWriter{
		func() *tlog.ConsoleWriter {
			w := tlog.NewConsoleWriter(tlog.Stderr, tlog.LstdFlags)
			w.IDWidth = 10

			return w
		}(),
	}, w)
}

func TestFileExtReader(t *testing.T) {
	OpenFileReader = func(n string, f int, m os.FileMode) (io.Reader, error) {
		return testFile(n), nil
	}
	CompressorBlockSize = 1 * compress.KiB

	r, err := OpenReader("stdin")
	assert.NoError(t, err)
	assert.Equal(t, tlio.NopCloser{
		Reader: tlog.Stdin,
	}, r)

	r, err = OpenReader("./stdin")
	assert.NoError(t, err)
	assert.Equal(t, testFile("./stdin"), r)

	r, err = OpenReader(".tlog.ez")
	assert.NoError(t, err)
	assert.Equal(t, tlio.NopCloser{Reader: compress.NewDecoder(tlog.Stdin)}, r)
}

func (testFile) Write(p []byte) (int, error) { return len(p), nil }

func (testFile) Read(p []byte) (int, error) { return 0, errors.New("test mock") }

func (testFile) Close() error { return nil }
