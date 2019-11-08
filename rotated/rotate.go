package rotated

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type (
	File struct {
		f       *os.File
		nbytes  int
		MaxSize int // 1 GiB

		name string

		Fallback io.Writer // os.Stderr
	}
)

var ( // testable time, rand
	now = time.Now
)

func NewFile(n string) *File {
	return &File{
		name:     n,
		MaxSize:  1 << 30,
		Fallback: os.Stderr,
	}
}

func (w *File) Write(p []byte) (n int, err error) {
	if w.f == nil || w.nbytes+len(p) > w.MaxSize {
		err = w.rotate()
		if err != nil {
			if w.Fallback != nil {
				_, _ = w.Fallback.Write([]byte("FAILED TO ROTATE FILE: " + err.Error() + "\n"))
				_, _ = w.Fallback.Write(p)
			}
			os.Exit(-1)
		}
	}

	n, err = w.f.Write(p)
	if err != nil {
		if w.Fallback != nil {
			_, _ = w.Fallback.Write([]byte("FAILED TO WRITE MESSAGE: " + err.Error() + "\n"))
			_, _ = w.Fallback.Write(p)
		}
		os.Exit(-1)
	}

	w.nbytes += n

	return n, err
}

func (w *File) rotate() (err error) {
	if w.f != nil {
		if err = w.f.Close(); err != nil {
			return err
		}
	}

	name := w.fname()

	w.f, err = os.Create(name)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "file created: %v\n", name)

	w.nbytes = 0

	return nil
}

func (w *File) fname() string {
	now := now()

	var name string
	if strings.Contains(w.name, "#") {
		name = strings.Replace(w.name, "#", now.Format(timeFormat), 1)
	} else {
		ext := filepath.Ext(w.name)
		name = strings.TrimSuffix(w.name, ext)
		name = name + "_" + now.Format(timeFormat) + ext
	}

	return name
}

func (w *File) Close() error {
	if w.f == nil {
		return nil
	}
	return w.f.Close()
}
