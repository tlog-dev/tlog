package tlflag

import (
	"os"
	"testing"

	"github.com/nikandfor/assert"
	"tlog.app/go/eazy"
	"tlog.app/go/errors"

	"tlog.app/go/tlog"
	"tlog.app/go/tlog/convert"
	"tlog.app/go/tlog/rotating"
	"tlog.app/go/tlog/tlio"
)

type testFile string

func TestingFileOpener(n string, f int, m os.FileMode) (interface{}, error) {
	return testFile(n), nil
}

func TestFileWriter(t *testing.T) {
	OpenFileWriter = TestingFileOpener

	w, err := OpenWriter("stderr")
	assert.NoError(t, err)
	assert.Equal(t, tlio.MultiWriter{
		tlog.NewConsoleWriter(tlog.Stderr, tlog.LstdFlags),
	}, w)

	w, err = OpenWriter("stderr?console=dm,stderr?console=dm")
	assert.NoError(t, err)
	assert.Equal(t, tlio.MultiWriter{
		tlog.NewConsoleWriter(tlog.Stderr, tlog.LdetFlags|tlog.Lmilliseconds),
		tlog.NewConsoleWriter(tlog.Stderr, tlog.LdetFlags|tlog.Lmilliseconds),
	}, w)

	w, err = OpenWriter(".tl,-.tl")
	assert.NoError(t, err)
	assert.Equal(t, tlio.MultiWriter{
		tlio.NopCloser{Writer: tlog.Stderr},
		tlio.NopCloser{Writer: tlog.Stdout},
	}, w)

	w, err = OpenWriter(".tlz")
	assert.NoError(t, err)
	assert.Equal(t, tlio.MultiWriter{
		eazy.NewWriter(tlog.Stderr, EazyBlockSize, EazyHTable),
	}, w)

	w, err = OpenWriter("file.tl")
	assert.NoError(t, err)
	assert.Equal(t, testFile("file.tl"), w)

	w, err = OpenWriter("file.tlz")
	assert.NoError(t, err)
	assert.Equal(t, tlio.WriteCloser{
		Writer: eazy.NewWriter(testFile("file.tlz"), EazyBlockSize, EazyHTable),
		Closer: testFile("file.tlz"),
	}, w)

	w, err = OpenWriter("file.tl.ez")
	assert.NoError(t, err)
	assert.Equal(t, tlio.WriteCloser{
		Writer: eazy.NewWriter(testFile("file.tl.ez"), EazyBlockSize, EazyHTable),
		Closer: testFile("file.tl.ez"),
	}, w)

	w, err = OpenWriter("file.ezdump")
	assert.NoError(t, err)
	assert.Equal(t, tlio.WriteCloser{
		Writer: eazy.NewWriter(eazy.NewDumper(testFile("file.ezdump")), EazyBlockSize, EazyHTable),
		Closer: testFile("file.ezdump"),
	}, w)

	w, err = OpenWriter("file.json")
	assert.NoError(t, err)
	assert.Equal(t, tlio.WriteCloser{
		Writer: convert.NewJSON(testFile("file.json")),
		Closer: testFile("file.json"),
	}, w)

	w, err = OpenWriter("file.json.ez")
	assert.NoError(t, err)
	assert.Equal(t, tlio.WriteCloser{
		Writer: convert.NewJSON(
			eazy.NewWriter(
				testFile("file.json.ez"),
				EazyBlockSize, EazyHTable)),
		Closer: testFile("file.json.ez"),
	}, w)
}

func TestURLWriter(t *testing.T) { //nolint:dupl
	OpenFileWriter = func(n string, f int, m os.FileMode) (interface{}, error) {
		return testFile(n), nil
	}

	w, err := OpenWriter("relative/path.tlog")
	assert.NoError(t, err)
	assert.Equal(t, testFile("relative/path.tlog"), w)

	w, err = OpenWriter("file://relative/path.tlog")
	assert.NoError(t, err)
	assert.Equal(t, testFile("relative/path.tlog"), w)

	w, err = OpenWriter("file:///absolute/path.tlog")
	assert.NoError(t, err)
	assert.Equal(t, testFile("/absolute/path.tlog"), w)
}

func TestRotatedWriter(t *testing.T) {
	OpenFileWriter = TestingFileOpener

	const CompressorBlockSize = 1 * eazy.MiB

	with := func(f *rotating.File, wrap func(*rotating.File)) *rotating.File {
		wrap(f)

		return f
	}

	w, err := OpenWriter("file_XXXX.tl")
	assert.NoError(t, err)
	assert.Equal(t, with(rotating.Create("file_XXXX.tl"), func(f *rotating.File) {
		f.OpenFile = openFileWriter
	}), w)

	w, err = OpenWriter("file.tl?rotating=1")
	assert.NoError(t, err)
	assert.Equal(t, with(rotating.Create("file.tl"), func(f *rotating.File) {
		f.OpenFile = openFileWriter
	}), w)

	w, err = OpenWriter("file_XXXX.tlz")
	assert.NoError(t, err)
	assert.Equal(t, with(rotating.Create("file_XXXX.tlz"), func(f *rotating.File) {
		f.OpenFile = RotatedTLZFileOpener(nil)
	}), w)

	w, err = OpenWriter("file_XXXX.tl.ez")
	assert.NoError(t, err)
	assert.Equal(t, with(rotating.Create("file_XXXX.tl.ez"), func(f *rotating.File) {
		f.OpenFile = RotatedTLZFileOpener(nil)
	}), w)

	w, err = OpenWriter("file_XXXX.json.ez")
	assert.NoError(t, err)
	assert.Equal(t, tlio.WriteCloser{
		Writer: convert.NewJSON(with(rotating.Create("file_XXXX.json.ez"), func(f *rotating.File) {
			f.OpenFile = RotatedTLZFileOpener(nil)
		})),
		Closer: with(rotating.Create("file_XXXX.json.ez"), func(f *rotating.File) {
			f.OpenFile = RotatedTLZFileOpener(nil)
		}),
	}, w)
}

func TestFileReader(t *testing.T) {
	OpenFileReader = TestingFileOpener

	r, err := OpenReader("stdin")
	assert.NoError(t, err)
	assert.Equal(t, tlio.NopCloser{
		Reader: os.Stdin,
	}, r)

	r, err = OpenReader("./stdin")
	assert.NoError(t, err)
	assert.Equal(t, testFile("./stdin"), r)

	r, err = OpenReader(".tlog.ez")
	assert.NoError(t, err)
	assert.Equal(t, tlio.NopCloser{Reader: eazy.NewReader(os.Stdin)}, r)
}

func TestURLReader(t *testing.T) { //nolint:dupl
	OpenFileReader = func(n string, f int, m os.FileMode) (interface{}, error) {
		return testFile(n), nil
	}

	w, err := OpenReader("relative/path.tlog")
	assert.NoError(t, err)
	assert.Equal(t, testFile("relative/path.tlog"), w)

	w, err = OpenReader("file://relative/path.tlog")
	assert.NoError(t, err)
	assert.Equal(t, testFile("relative/path.tlog"), w)

	w, err = OpenReader("file:///absolute/path.tlog")
	assert.NoError(t, err)
	assert.Equal(t, testFile("/absolute/path.tlog"), w)
}

func (testFile) Write(p []byte) (int, error) { return len(p), nil }

func (testFile) Read(p []byte) (int, error) { return 0, errors.New("test mock") }

func (testFile) Close() error { return nil }
