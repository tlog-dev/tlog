package tlog

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRotatedFile(t *testing.T) {
	dir, err := ioutil.TempDir("/tmp", "tlog_rotate_")
	if err != nil {
		t.Fatalf("create tmp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	f := NewFile(path.Join(dir, fmt.Sprintf("file_#.%d.log", os.Getpid())))
	defer f.Close()
	f.MaxSize = 20

	l := New(NewConsoleWriter(f, LstdFlags))

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

func TestRotatedMmap(t *testing.T) {
	dir, err := ioutil.TempDir("/tmp", "tlog_rotate_")
	if err != nil {
		t.Fatalf("create tmp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	f := NewMmapFile(path.Join(dir, fmt.Sprintf("file_#.%d.log", os.Getpid())), 50)

	l := New(NewConsoleWriter(f, LstdFlags))

	l.Printf("some info %v %v", "qweqweqew", 1)
	l.Printf("some info %v %v", "qweqweqew", 2)
	l.Printf("some info %v %v", "qweqweqew", 3)

	assert.NoError(t, f.Close())

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

		exp := fmt.Sprintf("some info qweqweqew %d\n", i+1)
		assert.Equal(t, exp, string(b[21:]))
	}
}
