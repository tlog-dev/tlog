package rotated

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFile(t *testing.T) {
	dir, err := ioutil.TempDir("", "tlog_rotate_")
	if err != nil {
		t.Fatalf("create tmp dir: %v", err)
	}
	defer func() {
		if !t.Failed() {
			os.RemoveAll(dir)
			return
		}

		t.Logf("dir: %v", dir)
	}()

	f := Create(filepath.Join(dir, fmt.Sprintf("file_@.%d.log", os.Getpid())))
	defer f.Close()
	f.MaxSize = 20

	for i := 0; i < 3; i++ {
		_, err = fmt.Fprintf(f, "some info %v %v\n", os.Args, i)
		assert.NoError(t, err)
	}

	fs, err := ioutil.ReadDir(dir)
	if err != nil {
		t.Fatalf("list dir: %v", err)
	}

	assert.Len(t, fs, 3)

	for i, f := range fs {
		b, err := ioutil.ReadFile(path.Join(dir, f.Name()))
		if err != nil {
			t.Fatalf("read file: %v", err)
		}

		assert.Equal(t, fmt.Sprintf("some info %v %v\n", os.Args, i), string(b))
	}
}

type ew struct {
	w  io.Writer
	we error
	ce error
}

func (w ew) Write(p []byte) (int, error) {
	if w.we != nil {
		return 0, w.we
	}

	return w.w.Write(p)
}

func (w ew) Close() error {
	if w.ce != nil {
		return w.ce
	}

	return nil
}

func TestFallbackOnErrors(t *testing.T) {
	n := 0
	var buf [3]bytes.Buffer

	f := Create("name")
	f.Fallback = &buf[0]
	f.Fopen = func(name string, mode os.FileMode) (io.Writer, error) {
		n++
		switch n {
		case 1:
			return nil, errors.New("open error")
		case 2:
			return ew{w: &buf[1], we: errors.New("write error")}, nil
		case 3:
			return ew{w: &buf[2], ce: errors.New("close error")}, nil
		default:
			return nil, nil
		}
	}

	_, err := f.Write([]byte("qwe\n"))
	assert.EqualError(t, err, "open error")

	_, err = f.Write([]byte("asd\n"))
	assert.EqualError(t, err, "write error")

	err = f.Rotate()
	assert.NoError(t, err)

	err = f.Rotate()
	assert.NoError(t, err) // close error is not reported

	assert.Equal(t, `ROTATE FAILED: open error
qwe
WRITE FAILED: write error
asd
CLOSE FAILED: close error
`, buf[0].String())
}

func TestFname(t *testing.T) {
	tm, _ := time.Parse(timeFormat, timeFormat)

	n := fname(filepath.Join("some", "path", "to.suff.log"), tm, 0)
	assert.Equal(t, filepath.Join("some", "path", "to.suff_"+timeFormat+".log"), n)

	n = fname(filepath.Join("some", "path", "to.suff.log"), tm, 4)
	assert.Equal(t, filepath.Join("some", "path", "to.suff_"+timeFormat+"_4"+".log"), n)

	n = fname(filepath.Join("some", "path", "to_file"), tm, 0)
	assert.Equal(t, filepath.Join("some", "path", "to_file_"+timeFormat), n)

	n = fname(filepath.Join("some", "path", "to_@_file"), tm, 0)
	assert.Equal(t, filepath.Join("some", "path", "to_"+timeFormat+"_file"), n)

	n = fname(filepath.Join("some", "path", "to_@_file"), tm, 3)
	assert.Equal(t, filepath.Join("some", "path", "to_"+timeFormat+"_3"+"_file"), n)
}

func TestOpenClose(t *testing.T) {
	f := Create("qweqewqew")
	err := f.Close()
	assert.NoError(t, err)
}

func TestFallback(t *testing.T) {
	var buf bytes.Buffer

	fallback(nil, "test reason", errors.New("error XYZ"), nil)

	fallback(&buf, "test reason", errors.New("error XYZ"), nil)

	assert.Equal(t, "test reason: error XYZ\n", buf.String())

	buf.Reset()

	fallback(&buf, "test reason", errors.New("error XYZ"), []byte("original message"))

	assert.Equal(t, "test reason: error XYZ\noriginal message", buf.String())
}
