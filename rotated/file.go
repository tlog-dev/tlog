package rotated

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type (
	File struct {
		mu sync.Mutex
		w  io.Writer

		name string

		size    int64
		rotated bool

		MaxSize int64

		ErrorOnRotate error

		Flags    int
		Mode     os.FileMode
		OpenFile func(name string, flag int, mode os.FileMode) (io.Writer, error)
	}

	RotatedError struct{}
)

var TimeFormat = "2006-01-02_15-04-05.000"

func Create(name string) (f *File) {
	f = &File{
		name:          name,
		MaxSize:       1 << 29, // 128MB
		ErrorOnRotate: RotatedError{},

		Flags:    os.O_CREATE | os.O_APPEND | os.O_WRONLY,
		Mode:     0544,
		OpenFile: OpenFileTimeSubst,
	}

	return f
}

func (f *File) Write(p []byte) (n int, err error) {
	defer f.mu.Unlock()
	f.mu.Lock()

	if f.w == nil || f.size+int64(len(p)) > f.MaxSize && f.size != 0 && f.MaxSize != 0 {
		err = f.rotate()
		if err != nil {
			return 0, err
		}
	}

	if f.rotated && f.ErrorOnRotate != nil {
		f.rotated = false

		return 0, f.ErrorOnRotate
	}

	f.rotated = false

	n, err = f.w.Write(p)
	f.size += int64(n)
	if err != nil {
		return
	}

	return
}

func (f *File) Rotate() error {
	defer f.mu.Unlock()
	f.mu.Lock()

	return f.rotate()
}

func (f *File) rotate() (err error) {
	if c, ok := f.w.(io.Closer); ok {
		_ = c.Close()
	}

	f.w, err = f.OpenFile(f.name, f.Flags, f.Mode)
	f.size = 0
	f.rotated = true

	return
}

func OpenFileOS(n string, f int, mode os.FileMode) (io.Writer, error) {
	return os.OpenFile(n, f, mode)
}

func OpenFileTimeSubst(base string, f int, mode os.FileMode) (io.Writer, error) {
	now := time.Now()

	name := fnameTime(base, now)

	return os.OpenFile(name, f, mode)
}

func fnameTime(name string, now time.Time) string {
	uniq := now.Format(TimeFormat)

	if p := strings.LastIndexByte(name, '@'); p != -1 {
		return name[:p] + uniq + name[p+1:]
	}

	ext := filepath.Ext(name)
	name = strings.TrimSuffix(name, ext)

	return name + "_" + uniq + ext
}

func (RotatedError) Error() string { return "file rotated" }

func (RotatedError) IsRotated() bool { return true }
