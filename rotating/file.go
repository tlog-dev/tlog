package rotating

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nikandfor/errors"
)

type (
	File struct {
		mu sync.Mutex
		w  io.Writer

		name            string
		current         string
		dir, pref, suff string
		format          string
		num             int

		size  int64
		start time.Time

		MaxFileSize int64
		MaxFileAge  time.Duration

		MaxTotalSize  int64
		MaxTotalAge   time.Duration
		MaxTotalFiles int // including current

		//	SubstPattern string

		Flags int
		Mode  os.FileMode

		OpenFile FileOpener                               `deep:"compare=pointer"`
		readdir  func(name string) ([]fs.DirEntry, error) `deep:"-"`
		fstat    func(name string) (fs.FileInfo, error)   `deep:"-"`
		remove   func(name string) error                  `deep:"-"`
		symlink  func(target, name string) error          `deep:"-"`

		removeSingleflight atomic.Bool
	}

	FileOpener = func(name string, flags int, mode os.FileMode) (io.Writer, error)
)

const (
	B = 1 << (iota * 10)
	KiB
	MiB
	GiB
	TiB

	KB = 1e3
	MB = 1e6
	GB = 1e9
	TB = 1e12
)

var (
	//	SubstPattern = "XXXX"
	//	TimeFormat   = "2006-01-02_15-04"

	patterns = []string{"xxx", "XXX", "ddd"}
)

func Create(name string) (f *File) {
	f = &File{
		name: name,

		MaxFileSize:   128 * MiB,
		MaxTotalAge:   28 * 24 * time.Hour,
		MaxTotalFiles: 10,

		//	SubstPattern: SubstPattern,
		//	TimeFormat: TimeFormat,

		OpenFile: openFile, //OpenFileTimeSubstWithSymlink,
		Flags:    os.O_CREATE | os.O_APPEND | os.O_WRONLY,
		Mode:     0o644,

		readdir: os.ReadDir,
		fstat:   os.Stat,
		remove:  os.Remove,
		symlink: os.Symlink,
	}

	return f
}

func (f *File) Write(p []byte) (n int, err error) {
	defer f.mu.Unlock()
	f.mu.Lock()

	if f.w == nil || f.size != 0 &&
		(f.MaxFileSize != 0 && f.size+int64(len(p)) > f.MaxFileSize ||
			f.MaxFileAge != 0 && time.Since(f.start) > f.MaxFileAge) {

		checkAgain := func() bool {
			// if triggered not by size
			if f.w == nil || f.size != 0 && f.MaxFileAge != 0 && time.Since(f.start) > f.MaxFileAge {
				return true
			}

			if f.current == "" || f.MaxFileSize == 0 {
				return true
			}

			inf, err := f.fstat(f.current)
			if err != nil {
				return true
			}

			if inf.Size()+int64(len(p)) > f.MaxFileSize {
				//	println("fine, rotate now")
				return true
			}

			//	println("we have only", inf.Size(), "bytes file, not", f.size, ". use it more")

			f.size = inf.Size()

			return false
		}

		if checkAgain() {
			err = f.rotate()
			if err != nil {
				return 0, errors.Wrap(err, "rotate")
			}
		}
	}

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
	if f.format == "" {
		f.dir, f.pref, f.suff, f.format = splitPattern(f.name)

		f.num, err = f.findMaxNum(f.dir, f.pref, f.suff, f.format)
		if err != nil {
			return errors.Wrap(err, "find max num")
		}
	}

	f.num++
	base := fmt.Sprintf("%s%s%s", f.pref, fmt.Sprintf(f.format, f.num), f.suff)
	fname := filepath.Join(f.dir, base)

	if c, ok := f.w.(io.Closer); ok {
		_ = c.Close()
	}

	f.w, err = f.OpenFile(fname, f.Flags, f.Mode)
	if err != nil {
		return errors.Wrap(err, "")
	}

	now := time.Now()

	f.current = fname
	f.size = 0
	f.start = fileCtime(f.fstat, fname, now)

	if f.symlink != nil {
		link := filepath.Join(f.dir, f.pref+"LATEST"+f.suff)

		_ = f.remove(link)
		_ = f.symlink(base, link)
	}

	if f.MaxTotalSize != 0 || f.MaxTotalAge != 0 || f.MaxTotalFiles != 0 {
		go f.removeOld(f.dir, base, f.pref, f.suff, f.format, f.start)
	}

	return
}

func (f *File) removeOld(dir, base, pref, suff, format string, now time.Time) error {
	if f.removeSingleflight.Swap(true) {
		return errors.New("already running")
	}

	defer f.removeSingleflight.Store(false)

	files, err := f.matchingFiles(dir, pref, suff, format)
	if err != nil {
		return err
	}

	files, err = f.filesToRemove(dir, base, now, files)
	if err != nil {
		return err
	}

	for _, name := range files {
		n := filepath.Join(dir, name)

		e := f.remove(n)
		if err == nil {
			err = errors.Wrap(e, "remove %v", name)
		}
	}

	return nil
}

func (f *File) findMaxNum(dir, pref, suff, format string) (num int, err error) {
	entries, err := f.readdir(dir)
	if err != nil {
		return 0, errors.Wrap(err, "read dir")
	}

	for _, e := range entries {
		n := e.Name()

		m, ok := f.parseName(n, pref, suff, format)
		if !ok {
			continue
		}

		if m > num {
			num = m
		}
	}

	return num, nil
}

func (f *File) matchingFiles(dir, pref, suff, format string) ([]string, error) {
	entries, err := f.readdir(dir)
	if err != nil {
		return nil, errors.Wrap(err, "read dir")
	}

	var files []string

	for _, e := range entries {
		n := e.Name()

		_, ok := f.parseName(n, pref, suff, format)
		if !ok {
			continue
		}

		files = append(files, n)
	}

	return files, nil
}

func (f *File) parseName(n, pref, suff, format string) (num int, ok bool) {
	//	defer func() { println("parse name", n, pref, suff, num, ok, loc.Caller(1).String()) }()
	if !strings.HasPrefix(n, pref) || !strings.HasSuffix(n, suff) || n == pref+"LATEST"+suff {
		return 0, false
	}

	uniq := n[len(pref) : len(n)-len(suff)]

	if _, err := fmt.Sscanf(uniq, format, &num); err != nil {
		return 0, false
	}

	return num, true
}

func (f *File) filesToRemove(dir, base string, now time.Time, files []string) ([]string, error) {
	p := len(files)

	for p > 0 && files[p-1] >= base {
		p--
	}

	//	tlog.Printw("files to remove", "past", files[:p], "future", files[p:])

	files = files[:p]
	size := int64(0)

	for p > 0 {
		prev := p - 1

		if f.MaxTotalFiles != 0 && len(files)-prev+1 > f.MaxTotalFiles {
			//	tlog.Printw("remove files", "reason", "max_total_files", "x", len(files)-prev, "of", f.MaxTotalFiles, "files", files[:p])
			break
		}

		n := filepath.Join(dir, files[prev])

		inf, err := f.fstat(n)
		if err != nil {
			return nil, errors.Wrap(err, "stat %v", files[prev])
		}

		size += inf.Size()

		if f.MaxTotalSize != 0 && size > f.MaxTotalSize {
			//	tlog.Printw("remove files", "reason", "max_total_size", "total_size", size, "of", f.MaxTotalSize, "files", files[:p])
			break
		}

		if f.MaxTotalAge != 0 && now.Sub(ctime(inf, now)) > f.MaxTotalAge {
			//	tlog.Printw("remove files", "reason", "max_total_age", "total_age", now.Sub(ctime(inf, now)), "of", f.MaxTotalAge, "files", files[:p])
			break
		}

		p--
	}

	return files[:p], nil
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

func IsPattern(name string) bool {
	_, pos := findPattern(name)

	return pos != -1
}

func splitPattern(name string) (dir, pref, suff, format string) {
	dir = filepath.Dir(name)
	base := filepath.Base(name)

	pattern, pos := findPattern(base)

	if pos == -1 {
		suff = filepath.Ext(base)
		pref = strings.TrimSuffix(base, suff)
		format = "%04X"

		return
	}

	l := len(pattern)

	for pos > 0 && base[pos-1] == pattern[0] {
		pos--
		l++
	}

	pref, suff = base[:pos], base[pos+l:]
	format = fmt.Sprintf("%%0%d%c", l, pattern[0])

	return
}

func findPattern(name string) (string, int) {
	var pattern string
	var pos int = -1

	for _, pat := range patterns {
		p := strings.LastIndex(name, pat)
		if p > pos {
			pos = p
			pattern = pat
		}
	}

	return pattern, pos
}

func openFile(name string, flags int, mode os.FileMode) (io.Writer, error) {
	return os.OpenFile(name, flags, mode)
}
