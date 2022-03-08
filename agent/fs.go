package agent

import (
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sync"

	"github.com/nikandfor/errors"
	"github.com/nikandfor/tlog"
)

type (
	OpenFiler interface {
		OpenFile(string, int, os.FileMode) (io.ReadWriteCloser, error)
	}

	OSFS struct {
		root string
	}

	MemFS struct {
		mu sync.Mutex

		files map[string]*memfile

		log *tlog.Logger
	}

	memfile struct {
		fs *MemFS

		name string
		mode os.FileMode
		d    []byte
	}

	openfile struct {
		f     *memfile
		pos   int
		flags int
		mode  string

		fs.FileInfo
	}
)

// root is os path with os specific path separator
func NewOSFS(root string) OSFS {
	return OSFS{root: root}
}

func (ff OSFS) Sub(dir string) (_ fs.FS, err error) {
	if err = ff.check("sub", dir); err != nil {
		return OSFS{}, err
	}

	return OSFS{root: filepath.Join(ff.root, dir)}, nil
}

func (ff OSFS) OpenFile(name string, flags int, mode os.FileMode) (_ io.ReadWriteCloser, err error) {
	if err = ff.check("open", name); err != nil {
		return nil, err
	}

	fname := ff.fname(name)

	if flags|os.O_CREATE != 0 {
		dir := filepath.Dir(fname)

		err = os.MkdirAll(dir, 0755)
		if err != nil {
			return nil, errors.Wrap(err, "mkdir")
		}
	}

	return os.OpenFile(fname, flags, mode)
}

func (ff OSFS) Open(name string) (fs.File, error) {
	if err := ff.check("open", name); err != nil {
		return nil, err
	}

	return os.Open(ff.fname(name))
}

func (ff OSFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if err := ff.check("readdir", name); err != nil {
		return nil, err
	}

	return os.ReadDir(ff.fname(name))
}

func (ff OSFS) check(op, name string) error {
	if !fs.ValidPath(name) {
		return &fs.PathError{
			Op:   op,
			Path: name,
			Err:  fs.ErrInvalid,
		}
	}

	return nil
}

func (ff OSFS) fname(name string) string {
	osname := filepath.FromSlash(name)
	return filepath.Join(ff.root, osname)
}

func NewMemFS() *MemFS {
	return &MemFS{
		files: make(map[string]*memfile),
	}
}

func (ff *MemFS) OpenFile(name string, flags int, mode os.FileMode) (_ io.ReadWriteCloser, err error) {
	return ff.openFile(name, flags, mode)
}

func (ff *MemFS) Open(name string) (fs.File, error) {
	return ff.openFile(name, os.O_RDONLY, 0)
}

func (ff *MemFS) openFile(name string, flags int, mode os.FileMode) (_ *openfile, err error) {
	if err = ff.check("open", name); err != nil {
		return nil, err
	}

	if !(flags&os.O_RDONLY == os.O_RDONLY || flags&os.O_WRONLY == os.O_WRONLY || flags&os.O_RDWR == os.O_RDWR) {
		return nil, errors.New("bad open mode")
	}

	name = path.Clean(name)

	f, ok := ff.files[name]
	if !ok && flags|os.O_CREATE == 0 {
		return nil, fs.ErrNotExist
	}

	if !ok {
		f = &memfile{
			fs:   ff,
			name: name,
			mode: mode,
		}

		ff.files[name] = f
	}

	o := &openfile{
		f:     f,
		flags: flags,
		mode:  "_",
	}

	switch {
	case flags&os.O_WRONLY == os.O_WRONLY:
		o.mode = "w"
	case flags&os.O_RDWR == os.O_RDWR:
		o.mode = "rw"
	case flags&os.O_RDONLY == os.O_RDONLY:
		o.mode = "r"
	}

	ff.log.Printw("open file", "name", name, "mode", o.mode, "fsize", len(f.d))

	return o, nil
}

func (ff *MemFS) check(op, name string) error {
	if !fs.ValidPath(name) {
		return &fs.PathError{
			Op:   op,
			Path: name,
			Err:  fs.ErrInvalid,
		}
	}

	return nil
}

func (o *openfile) Read(p []byte) (n int, err error) {
	defer func() {
		o.f.fs.log.Printw("read file", "name", o.f.name, "pos", o.pos-n, "n", n, "err", err)
	}()

	if o.mode != "r" && o.mode != "rw" {
		return 0, errors.New("write only")
	}

	defer o.f.fs.mu.Unlock()
	o.f.fs.mu.Lock()

	n = copy(p, o.f.d[o.pos:])
	o.pos += n

	if o.pos == len(o.f.d) {
		err = io.EOF
	}

	return
}

func (o *openfile) Write(p []byte) (n int, err error) {
	defer func() {
		o.f.fs.log.Printw("write file", "name", o.f.name, "pos", o.pos-n, "n", n, "err", err)
	}()

	if o.mode != "w" && o.mode != "rw" {
		return 0, errors.New("read only")
	}

	defer o.f.fs.mu.Unlock()
	o.f.fs.mu.Lock()

	o.f.d = append(o.f.d[:o.pos], p...)
	n = len(p)
	o.pos += n

	return
}

func (o *openfile) Close() error {
	return nil
}

func (o *openfile) Stat() (fs.FileInfo, error) {
	return o, nil
}

func (o *openfile) Name() string {
	return o.f.name
}

func (o *openfile) Size() int64 {
	return int64(len(o.f.d))
}

func (o *openfile) IsDir() bool {
	return false
}
