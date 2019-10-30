package tlog

import (
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"syscall"
)

type (
	File struct {
		f       *os.File
		nbytes  int
		MaxSize int // 1 GiB

		name string

		Fallback io.Writer // os.Stderr
	}

	Mmap struct {
		mu     sync.Mutex
		b      []byte
		nbytes int
		f      *os.File

		max  int
		name string

		Fallback io.Writer // os.Stderr
	}
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

	now := now()

	var name string
	if strings.Contains(w.name, "#") {
		name = strings.Replace(w.name, "#", now.Format("2006-01-02_15:04:05.000000_07:00"), 1)
	} else {
		name = w.name + "_" + now.Format("2006-01-02_15:04:05.000000_07:00")
	}

	w.f, err = os.Create(name)
	if err != nil {
		return err
	}

	w.nbytes = 0

	return nil
}

func (w *File) Close() error {
	if w.f == nil {
		return nil
	}
	return w.f.Close()
}

func NewMmapFile(n string, max int) *Mmap {
	return &Mmap{
		max:      max,
		name:     n,
		Fallback: os.Stderr,
	}
}

func (w *Mmap) Write(p []byte) (n int, err error) {
	defer w.mu.Unlock()
	w.mu.Lock()

	if w.b == nil || w.nbytes+len(p) > w.max {
		err = w.rotate(len(p))
		if err != nil {
			if w.Fallback != nil {
				_, _ = w.Fallback.Write([]byte("FAILED TO ROTATE FILE: " + err.Error() + "\n"))
				_, _ = w.Fallback.Write(p)
			}
			os.Exit(-1)
		}
	}

	n = copy(w.b[w.nbytes:], p)
	w.nbytes += n

	return n, nil
}

func (w *Mmap) rotate(s int) (err error) {
	if err = w.close(); err != nil {
		return err
	}

	if s > w.max {
		return errors.New("write bigger than mmap size")
	}

	now := now()

	var name string
	if strings.Contains(w.name, "#") {
		name = strings.Replace(w.name, "#", now.Format("2006-01-02_15:04:05.000000_07:00"), 1)
	} else {
		name = w.name + "_" + now.Format("2006-01-02_15:04:05.000000_07:00")
	}

	w.f, err = os.Create(name)
	if err != nil {
		return err
	}

	err = w.f.Truncate(int64(w.max))
	if err != nil {
		w.f.Close()
		return err
	}

	ff := syscall.MAP_SHARED
	if w.max > 1<<30 {
		ff |= syscall.MAP_HUGETLB
	}
	w.b, err = syscall.Mmap(int(w.f.Fd()), 0, w.max, syscall.PROT_WRITE, ff)
	if err != nil {
		w.f.Close()
		return err
	}

	w.nbytes = 0

	return nil
}

func (w *Mmap) Close() error {
	defer w.mu.Unlock()
	w.mu.Lock()
	return w.close()
}

func (w *Mmap) close() (err error) {
	if w.b == nil {
		return nil
	}
	defer func() {
		if e := w.f.Close(); err == nil {
			err = e
		}
	}()

	b := w.b
	w.b = nil
	if err = syscall.Munmap(b); err != nil {
		return
	}

	if err = syscall.Ftruncate(int(w.f.Fd()), int64(w.nbytes)); err != nil {
		return
	}

	return
}
