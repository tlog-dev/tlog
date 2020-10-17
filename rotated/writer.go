package rotated

import (
	"errors"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nikandfor/tlog"
	"gopkg.in/fsnotify.v1"
)

const timeFormat = "2006-01-02_15-04-05.000"

var SubstituteSymbol = '@'

type (
	Writer struct {
		mu     sync.Mutex
		w      io.Writer
		n      string
		nbytes int

		fsw *fsnotify.Watcher

		name string
		Flag int
		Mode os.FileMode

		MaxSize   int                                               // 1GB
		Fallback  io.Writer                                         // os.Stderr
		Fopen     func(string, int, os.FileMode) (io.Writer, error) // os.OpenFile
		NameSubst func(string) string                               // substitute @ -> current timestamp

		stopc chan struct{}
	}

	Option func(w *Writer)
)

var ErrRetry = errors.New("retry")

var now = time.Now

var tl *tlog.Logger

func SetTestLogger(l *tlog.Logger) { tl = l }

func NewWriter(name string, flag int, mode os.FileMode, ops ...Option) (*Writer, error) {
	w := &Writer{
		name:      name,
		Flag:      flag,
		Mode:      mode,
		MaxSize:   1 << 30, // 1Gb
		Fallback:  os.Stderr,
		Fopen:     osFopen,
		NameSubst: TimestampSubst,
	}

	for _, o := range ops {
		o(w)
	}

	if w.Flag == 0 {
		w.Flag = os.O_CREATE | os.O_EXCL | os.O_APPEND | os.O_WRONLY
	}
	if w.Mode == 0 {
		w.Mode = 0644
	}

	err := w.rotate()
	if err != nil {
		return nil, err
	}

	return w, nil
}

func (w *Writer) Write(p []byte) (n int, err error) {
	defer w.mu.Unlock()
	w.mu.Lock()

	if w.w == nil || w.MaxSize != 0 && w.nbytes+len(p) > w.MaxSize {
		err = w.rotate()
		if err != nil {
			fallback(w.Fallback, "ROTATE FAILED", err, p)

			return 0, err
		}
	}

	n, err = w.w.Write(p)
	w.nbytes += n
	if err != nil {
		fallback(w.Fallback, "WRITE FAILED", err, p)

		return n, err
	}

	return
}

func (w *Writer) Rotate() (err error) {
	w.mu.Lock()
	err = w.rotate()
	w.mu.Unlock()

	return err
}

func (w *Writer) rotate() (err error) {
	if c, ok := w.w.(io.Closer); ok {
		if err = c.Close(); err != nil {
			fallback(w.Fallback, "CLOSE FAILED", err, nil)
		}
	}

	w.nbytes = 0

again:
	w.n = w.name
	if f := w.NameSubst; f != nil {
		w.n = f(w.name)
	}

	w.w, err = w.Fopen(w.n, w.Flag, w.Mode)
	if errors.Is(err, ErrRetry) {
		goto again
	}

	if w.w != nil {
		f, ok := w.w.(interface {
			Name() string
		})

		if ok {
			w.n = f.Name()
		}
	}

	return
}

func (w *Writer) Close() (err error) {
	if c, ok := w.w.(io.Closer); ok {
		err = c.Close()
	}

	w.n = ""
	w.w = nil

	close(w.stopc)

	return
}

func (w *Writer) RotateOnSignal(sig ...os.Signal) {
	c := make(chan os.Signal, 3)
	signal.Notify(c, sig...)

	go func() {
		for {
			select {
			case <-c:
			case <-w.stopc:
				return
			}

			_ = w.Rotate()
		}
	}()
}

func (w *Writer) RotateOnFileMoved() (err error) {
	defer w.mu.Unlock()
	w.mu.Lock()

	if w.fsw != nil {
		panic("multiple watchers started")
	}

	w.fsw, err = fsnotify.NewWatcher()
	if err != nil {
		return
	}

	if w.n == "" {
		panic("file is not open")
	}

	err = w.fsw.Add(w.n)
	if err != nil {
		return
	}

	cl := func() {
		defer w.mu.Unlock()
		w.mu.Lock()

		_ = w.fsw.Close()
		w.fsw = nil
	}

	go func() {
		defer cl()

		var ev fsnotify.Event

		for {
			select {
			case ev = <-w.fsw.Events:
			case <-w.fsw.Errors:
				return
			case <-w.stopc:
				return
			}

			if ev.Op != fsnotify.Rename {
				continue
			}

			_ = w.Rotate()
		}
	}()

	return nil
}

func (w *Writer) Name() string {
	return w.name
}

func osFopen(n string, ff int, m os.FileMode) (io.Writer, error) {
	f, err := os.OpenFile(n, ff, m)
	if err != nil {
		return nil, err // prevent (*os.File)(nil) writer
	}

	return f, nil
}

func fopenTimeSubst(name string, flags int, mode os.FileMode) (io.Writer, error) {
	try := 10

again:
	now := now()

	n := fname(name, now)

	f, err := os.OpenFile(n, flags, mode)
	if os.IsExist(err) && try > 0 {
		try--
		goto again
	}

	return f, err
}

func fname(name string, now time.Time) string {
	uniq := now.Format(timeFormat)

	if p := strings.LastIndexByte(name, byte(SubstituteSymbol)); p != -1 {
		return name[:p] + uniq + name[p+1:]
	}

	ext := filepath.Ext(name)
	name = strings.TrimSuffix(name, ext)

	return name + "_" + uniq + ext
}

func TimestampSubst(n string) string {
	return fname(n, time.Now())
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

func WithFallback(w io.Writer) Option {
	return func(f *Writer) {
		f.Fallback = w
	}
}

func WithFopener(f func(string, int, os.FileMode) (io.Writer, error)) Option {
	return func(w *Writer) {
		w.Fopen = f
	}
}

func WithMaxSize(s int) Option {
	return func(w *Writer) {
		w.MaxSize = s
	}
}

func WithNameSubst(f func(n string) string) Option {
	return func(w *Writer) {
		w.NameSubst = f
	}
}

func WithFileMode(m os.FileMode) Option {
	return func(w *Writer) {
		w.Mode = m
	}
}
