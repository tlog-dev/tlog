package rotated

import (
	"bytes"
	"errors"
	"fmt"
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

	f := Create(filepath.Join(dir, fmt.Sprintf("file_#.%d.log", os.Getpid())))
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

func TestFname(t *testing.T) {
	tm, _ := time.Parse(timeFormat, timeFormat)

	n := fname(filepath.Join("some", "path", "to.suff.log"), tm, 0)
	assert.Equal(t, filepath.Join("some", "path", "to.suff_"+timeFormat+".log"), n)

	n = fname(filepath.Join("some", "path", "to.suff.log"), tm, 4)
	assert.Equal(t, filepath.Join("some", "path", "to.suff_"+timeFormat+"_4"+".log"), n)

	n = fname(filepath.Join("some", "path", "to_file"), tm, 0)
	assert.Equal(t, filepath.Join("some", "path", "to_file_"+timeFormat), n)

	n = fname(filepath.Join("some", "path", "to_#_file"), tm, 0)
	assert.Equal(t, filepath.Join("some", "path", "to_"+timeFormat+"_file"), n)

	n = fname(filepath.Join("some", "path", "to_#_file"), tm, 3)
	assert.Equal(t, filepath.Join("some", "path", "to_"+timeFormat+"_3"+"_file"), n)
}

func TestOpenClose(t *testing.T) {
	f := Create("qweqewqew")
	err := f.Close()
	assert.NoError(t, err)
}

func TestName(t *testing.T) {
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

	tm, _ := time.Parse(timeFormat, timeFormat)
	now = func() time.Time { return tm }

	f := Create(filepath.Join(dir, "qwe.log"))

	assert.Equal(t, "", f.Name()) // not open yet

	f.Write([]byte("something"))
	assert.Equal(t, "qwe_2006-01-02_15-04-05.log", filepath.Base(f.Name()))
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
