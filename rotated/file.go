package rotated

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const timeFormat = "2006-01-02_15-04-05.000"

type (
	File struct {
		mu     sync.Mutex
		f      io.Writer
		nbytes int

		name    string
		MaxSize int // 1 GB

		Fallback io.Writer // os.Stderr

		Mode  os.FileMode
		Fopen func(string, os.FileMode) (io.Writer, error)

		stopc chan struct{}
	}
)

var now = time.Now

func Create(name string) *File {
	return &File{
		name:     name,
		MaxSize:  1 << 30,
		Fallback: os.Stderr,
		Mode:     0440,
		Fopen:    FopenUniq,
		stopc:    make(chan struct{}),
	}
}

func (w *File) Write(p []byte) (int, error) {
	defer w.mu.Unlock()
	w.mu.Lock()

	if w.f == nil || w.nbytes+len(p) > w.MaxSize && w.MaxSize != 0 {
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

func (w *File) RotateOnSignal(sig ...os.Signal) {
	c := make(chan os.Signal, 3)
	signal.Notify(c, sig...)

	go w.rotator(c)
}

func (w *File) rotator(c chan os.Signal) {
	for {
		select {
		case <-c:
		case <-w.stopc:
			return
		}

		w.Rotate()
	}
}

func (w *File) Rotate() (err error) {
	defer w.mu.Unlock()
	w.mu.Lock()

	return w.rotate()
}

func (w *File) rotate() (err error) {
	if c, ok := w.f.(io.Closer); ok {
		if err = c.Close(); err != nil {
			fallback(w.Fallback, "CLOSE FAILED", err, nil)
		}
	}

	w.f, err = w.Fopen(w.name, w.Mode)

	w.nbytes = 0

	return err
}

func FopenSimple(name string, mode os.FileMode) (io.Writer, error) {
	return os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_APPEND, mode)
}

func FopenUniq(name string, mode os.FileMode) (io.Writer, error) {
	now := now()
	try := 0

again:
	n := fname(name, now, try)

	f, err := os.OpenFile(n, os.O_WRONLY|os.O_CREATE|os.O_APPEND|os.O_EXCL, mode)
	if os.IsExist(err) && try < 10 {
		try++
		goto again
	}

	return f, err
}

func fname(name string, now time.Time, try int) string {
	uniq := now.Format(timeFormat)
	if try != 0 {
		uniq += fmt.Sprintf("_%x", try)
	}

	if p := strings.LastIndexByte(name, '@'); p != -1 {
		return name[:p] + uniq + name[p+1:]
	}

	ext := filepath.Ext(name)
	name = strings.TrimSuffix(name, ext)
	return name + "_" + uniq + ext
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
	if c, ok := w.f.(io.Closer); ok {
		err = c.Close()
	}

	w.f = nil

	close(w.stopc)

	return
}
