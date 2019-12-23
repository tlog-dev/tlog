package rotated

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const timeFormat = "2006-01-02_15-04-05"

type (
	File struct {
		mu     sync.Mutex
		f      *os.File
		nbytes int

		name    string
		MaxSize int // 1 GiB

		Fallback io.Writer // os.Stderr

		Mode os.FileMode
	}
)

var now = time.Now

func Create(name string) *File {
	return &File{
		name:     name,
		MaxSize:  1 << 30,
		Fallback: os.Stderr,
		Mode:     0440,
	}
}

func (w *File) Write(p []byte) (int, error) {
	defer w.mu.Unlock()
	w.mu.Lock()

	if w.f == nil || w.nbytes+len(p) > w.MaxSize {
		err := w.rotate()
		if err != nil {
			fallback(w.Fallback, "ROTATE FAILED", err, p)
			return 0, err
		}
	}

	n, err := w.f.Write(p)
	if err != nil {
		fallback(w.Fallback, "WRITE FAILED", err, p)
		return n, err
	}

	w.nbytes += n

	return n, nil
}

func (w *File) Name() string {
	defer w.mu.Unlock()
	w.mu.Lock()

	if w.f == nil {
		return ""
	}

	return w.f.Name()
}

func (w *File) rotate() (err error) {
	if w.f != nil {
		if err = w.f.Close(); err != nil {
			fallback(w.Fallback, "CLOSE FAILED", err, nil)
		}
	}

	now := now()
	try := 0

again:
	name := fname(w.name, now, try)

	w.f, err = fopen(name, w.Mode)
	if os.IsExist(err) && try < 10 {
		try++
		goto again
	}
	if err != nil {
		return err
	}

	return nil
}

func fname(name string, now time.Time, try int) string {
	uniq := now.Format(timeFormat)
	if try != 0 {
		uniq += fmt.Sprintf("_%x", try)
	}

	if p := strings.LastIndexByte(name, '#'); p != -1 {
		return name[:p] + uniq + name[p+1:]
	}

	ext := filepath.Ext(name)
	name = strings.TrimSuffix(name, ext)
	return name + "_" + uniq + ext
}

func fopen(name string, mode os.FileMode) (*os.File, error) {
	return os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_APPEND|os.O_EXCL, mode)
}

func fallback(w io.Writer, r string, err error, msg []byte) {
	if w == nil {
		return
	}

	_, _ = w.Write([]byte(r + ": " + err.Error() + "\n"))

	if msg == nil {
		return
	}

	_, _ = w.Write(msg)
}

func (w *File) Close() (err error) {
	if w.f == nil {
		return nil
	}
	err = w.f.Close()
	w.f = nil
	return
}
