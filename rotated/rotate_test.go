package rotated

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/nikandfor/tlog"
	"github.com/stretchr/testify/assert"
)

func TestRotatedFile(t *testing.T) {
	dir, err := ioutil.TempDir("", "tlog_rotate_")
	if err != nil {
		t.Fatalf("create tmp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	f := NewFile(filepath.Join(dir, fmt.Sprintf("file_#.%d.log", os.Getpid())))
	defer f.Close()
	f.MaxSize = 20

	l := tlog.New(tlog.NewConsoleWriter(f, tlog.LstdFlags))

	l.Printf("some info %v %v", os.Args, 1)
	l.Printf("some info %v %v", os.Args, 2)
	l.Printf("some info %v %v", os.Args, 3)

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

		assert.Contains(t, string(b), "some info ")
		assert.Contains(t, string(b), fmt.Sprintf("%d\n", i+1))
	}
}

func TestRotatedFname(t *testing.T) {
	defer func(old func() time.Time) {
		now = old
	}(now)
	now = func() time.Time {
		t, _ := time.Parse("2006-01-02_15:04:05.000000_07:00", "2006-01-02_15:04:05.000000_07:00")
		return t
	}

	f := NewFile(filepath.Join("some", "path", "to.suff.log"))
	assert.Equal(t, filepath.Join("some", "path", "to.suff_"+timeFormat+".log"), f.fname())

	f = NewFile(filepath.Join("some", "path", "to_file"))
	assert.Equal(t, filepath.Join("some", "path", "to_file_"+timeFormat), f.fname())

	f = NewFile(filepath.Join("some", "path", "to_#_file"))
	assert.Equal(t, filepath.Join("some", "path", "to_"+timeFormat+"_file"), f.fname())
}

func TestRotatedOpenClose(t *testing.T) {
	f := NewFile("qweqewqew")
	err := f.Close()
	assert.NoError(t, err)
}
