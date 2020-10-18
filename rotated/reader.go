package rotated

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/fsnotify.v1"
)

type (
	Reader struct {
		r io.Reader
		n string
		w *fsnotify.Watcher

		name string

		Follow bool

		Fopen    func(string) (io.Reader, error) // os.Open
		NextName func(base, last string) (string, bool, error)

		stopc chan struct{}
	}

	ReaderOption func(r *Reader)
)

func NewReader(name string, ops ...ReaderOption) (r *Reader, err error) {
	r = &Reader{
		name:     name,
		Fopen:    func(n string) (io.Reader, error) { return os.Open(n) },
		NextName: NextName(true),
		stopc:    make(chan struct{}),
	}

	for _, o := range ops {
		o(r)
	}

	_, err = r.next()

	return r, err
}

func (r *Reader) Read(p []byte) (n int, err error) {
more:
	if r.r != nil {
		var rn int
		rn, err = r.r.Read(p[n:])
		//	tl.Printf("read %d %v  from %v of %v\n", rn, err, n, len(p))
		n += rn
		if err != io.EOF {
			return
		}
	}

	move, err := r.next()
	if err != nil {
		return
	}

	if !move {
		if n == 0 && r.Follow {
			err = r.wait()
			if err != nil {
				return
			}

			goto more
		}

		return n, io.EOF
	}

	if n < len(p) {
		goto more
	}

	return
}

func (r *Reader) Close() error {
	close(r.stopc)

	return nil
}

func (r *Reader) wait() (err error) {
	if r.w == nil {
		r.w, err = fsnotify.NewWatcher()
		if err != nil {
			return
		}

		err = r.w.Add(filepath.Dir(r.name))
		if err != nil {
			return
		}
	}

	cur := r.n

	err = r.w.Add(cur)
	if err != nil {
		return
	}

	defer func() {
		e := r.w.Remove(cur)
		if err == nil {
			err = e
		}
	}()

	select {
	case <-r.w.Events:
	case err = <-r.w.Errors:
		return
	case <-r.stopc:
		return io.ErrClosedPipe
	}

	return
}

func (r *Reader) next() (moved bool, err error) {
	next := r.name
	var move bool
	if f := r.NextName; f != nil {
		next, move, err = f(r.name, r.n)
	}
	if err != nil {
		return
	}
	if !move {
		return false, nil
	}

	//	tl.Printf("base %v  last %v  next %v\n", r.name, r.n, next)

	if c, ok := r.r.(io.Closer); ok {
		if err = c.Close(); err != nil {
			return
		}
	}

	r.r, err = r.Fopen(next)
	if err != nil {
		return
	}

	if r.r != nil {
		f, ok := r.r.(interface {
			Name() string
		})

		if ok {
			r.n = f.Name()
		}
	}

	return true, nil
}

func NextName(fwd bool) func(b, l string) (next string, ok bool, err error) {
	return func(b, l string) (next string, ok bool, err error) {
		var pref, ext string
		if p := strings.LastIndexByte(b, byte(SubstituteSymbol)); p != -1 {
			pref = b[:p]
			ext = b[p+1:]
		} else {
			ext = filepath.Ext(b)
			pref = strings.TrimSuffix(b, ext)
		}

		glob := pref + "*" + ext

		var fs []string
		fs, err = filepath.Glob(glob)
		if err != nil {
			return
		}

		for i := range fs {
			fs[i] = strings.TrimPrefix(fs[i], pref)
			fs[i] = strings.TrimSuffix(fs[i], ext)
		}

		sort.Slice(fs, func(i, j int) bool {
			if fwd {
				return cmp(fs[i], fs[j])
			} else {
				return cmp(fs[j], fs[i])
			}
		})

		x := strings.TrimPrefix(l, pref)
		x = strings.TrimSuffix(x, ext)
		var p int
		if l != "" {
			p = sort.Search(len(fs), func(i int) bool {
				if fwd {
					return !cmp(fs[i], x)
				} else {
					return !cmp(x, fs[i])
				}
			})
		}

		//	tl.Printf("base %q l %q p %v fs %q", b, x, p, fs)

		if p < len(fs) && pref+fs[p]+ext == l {
			p++
		}

		if p == len(fs) {
			return
		}

		return pref + fs[p] + ext, true, nil
	}
}

func cmp(a, b string) bool {
	an, aerr := strconv.ParseInt(a, 10, 64)
	bn, berr := strconv.ParseInt(b, 10, 64)

	if aerr == nil && berr == nil {
		return an < bn
	}

	return a < b
}

func Follow(r *Reader) {
	r.Follow = true
}
