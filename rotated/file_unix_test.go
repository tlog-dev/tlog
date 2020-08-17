// +build linux darwin

package rotated

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRotateBySignal(t *testing.T) {
	n := 0
	var buf [3]bytes.Buffer
	c := make(chan struct{}, 3)

	f := CreateLogrotate("name")
	f.Fopen = func(name string, mode os.FileMode) (io.Writer, error) { n++; c <- struct{}{}; return &buf[n-1], nil }

	q := make(chan os.Signal, 1)
	signal.Notify(q, syscall.SIGUSR1)

	_, _ = f.Write([]byte("before"))

	err := syscall.Kill(os.Getpid(), syscall.SIGUSR1)
	require.NoError(t, err)

	<-q

loop:
	for {
		select {
		case <-c:
		case <-time.After(100 * time.Millisecond):
			break loop
		}
	}

	_, _ = f.Write([]byte("after"))

	//	t.Logf("n: %v", n)
	assert.True(t, n >= 2)
	assert.Equal(t, "before", buf[0].String())
	assert.Equal(t, "after", buf[1].String())
	//	assert.Equal(t, "beforeafter", buf[0].String()+buf[1].String()+buf[2].String())
}

func TestFileLogrotate(t *testing.T) {
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

	fname := fmt.Sprintf("file.%d.log", os.Getpid())

	f := CreateLogrotate(filepath.Join(dir, fname))
	defer f.Close()

	for i := 0; i < 3; i++ {
		_, err = fmt.Fprintf(f, "some info %v\n", i)
		assert.NoError(t, err)

		err = os.Rename(
			filepath.Join(dir, fname),
			filepath.Join(dir, fmt.Sprintf("file_moved_%d.%d.log", i, os.Getpid())),
		)
		require.NoError(t, err)

		_, err = fmt.Fprintf(f, "after move %v\n", i)
		assert.NoError(t, err)

		err = f.Rotate()
		assert.NoError(t, err)
	}

	fs, err := ioutil.ReadDir(dir)
	if err != nil {
		t.Fatalf("list dir: %v", err)
	}

	assert.Len(t, fs, 4)

	for _, f := range fs {
		b, err := ioutil.ReadFile(path.Join(dir, f.Name()))
		if err != nil {
			t.Fatalf("read file: %v", err)
		}

		switch {
		case strings.HasPrefix(f.Name(), "file."):
			assert.Equal(t, "", string(b))
		case strings.HasPrefix(f.Name(), "file_moved_"):
			var n int
			fmt.Sscanf(f.Name(), "file_moved_%d", &n)
			assert.Equal(t, fmt.Sprintf("some info %v\nafter move %v\n", n, n), string(b))
		}
	}
}
