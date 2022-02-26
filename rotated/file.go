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

const (
	B = 1 << (iota * 10)
	KiB
	MiB
	GiB
)

var (
	SubstChar  = '@'
	TimeFormat = "2006-01-02_15-04"
)

func Create(name string) (f *File) {
	f = &File{
		name:    name,
		MaxSize: 128 * MiB,
		//	ErrorOnRotate: RotatedError{},

		Flags:    os.O_CREATE | os.O_APPEND | os.O_WRONLY,
		Mode:     0644,
		OpenFile: OpenFileTimeSubstWithSymlink,
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

func (f *File) Close() (err error) {
	defer f.mu.Unlock()
	f.mu.Lock()

	c, ok := f.w.(io.Closer)
	if ok {
		err = c.Close()
	}

	f.w = nil

	return err
}

func OpenFileOS(n string, f int, mode os.FileMode) (io.Writer, error) {
	return os.OpenFile(n, f, mode)
}

func OpenFileTimeSubst(base string, f int, mode os.FileMode) (w io.Writer, err error) {
	now := time.Now()

	name, _ := fnameTime(base, now)

	return os.OpenFile(name, f, mode)
}

func OpenFileTimeSubstWithSymlink(base string, f int, mode os.FileMode) (w io.Writer, err error) {
	now := time.Now()

	name, link := fnameTime(base, now)

	w, err = os.OpenFile(name, f, mode)
	if err != nil {
		return
	}

	_ = os.Remove(link)
	_ = os.Symlink(name, link)

	return
}

func fnameTime(name string, now time.Time) (f, l string) {
	uniq := now.Format(TimeFormat)

	if p := strings.LastIndexByte(name, byte(SubstChar)); p != -1 {
		f = name[:p] + uniq + name[p+1:]
		l = name[:p] + "LATEST" + name[p+1:]
		return
	}

	ext := filepath.Ext(name)
	name = strings.TrimSuffix(name, ext)

	f = name + "_" + uniq + ext
	l = name + "_" + "LATEST" + ext
	return
}

func (RotatedError) Error() string { return "file rotated" }
