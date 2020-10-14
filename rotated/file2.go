package rotated

import (
	"io"
	"os"
	"sync"
)

type (
	File2 struct {
		mu sync.Mutex
		w  io.Writer

		name string
		flag int
		perm os.FileMode

		Fallback io.Writer // os.Stderr
	}
)

func OpenFile(name string, flag int, perm os.FileMode) (*File2, error) {
	f, err := os.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}

	return &File2{
		w:    f,
		name: name,
		flag: flag,
		perm: perm,
	}, nil
}

func (f *File2) Write(p []byte) (n int, err error) {
	defer f.mu.Unlock()
	f.mu.Lock()

	if f.w == nil {
		err = f.rotate()
		if err != nil {
			fallback(f.Fallback, "ROTATE FAILED", err, p)

			return 0, err
		}
	}

	n, err = f.w.Write(p)
	if err != nil {
		fallback(f.Fallback, "WRITE FAILED", err, p)

		return n, err
	}

	return
}

func (f *File2) Rotate() (err error) {
	f.mu.Lock()
	err = f.rotate()
	f.mu.Unlock()

	return err
}

func (f *File2) rotate() (err error) {
	if c, ok := f.w.(io.Closer); ok {
		if err = c.Close(); err != nil {
			fallback(f.Fallback, "CLOSE FAILED", err, nil)
		}
	}

	f.w, err = os.OpenFile(f.name, f.flag, f.perm)

	return err
}

func (f *File2) Close() (err error) {
	if c, ok := f.w.(io.Closer); ok {
		err = c.Close()
	}

	f.w = nil

	//	close(f.stopc)

	return
}
