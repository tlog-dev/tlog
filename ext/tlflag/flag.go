package tlflag

import (
	"io"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nikandfor/errors"
	"github.com/nikandfor/loc"
	"tlog.app/go/eazy"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/convert"
	"github.com/nikandfor/tlog/rotated"
	"github.com/nikandfor/tlog/tlio"
	"github.com/nikandfor/tlog/tlwire"
)

type (
	FileOpener = func(name string, flags int, mode os.FileMode) (interface{}, error)

	writerWrapper func(io.Writer, io.Closer) (io.Writer, io.Closer, error)
	readerWrapper func(io.Reader, io.Closer) (io.Reader, io.Closer, error)
)

var (
	OpenFileWriter FileOpener = OSOpenFile
	OpenFileReader FileOpener = OpenFileReReader(OSOpenFile)
)

func OpenWriter(dst string) (wc io.WriteCloser, err error) {
	var ws tlio.MultiWriter

	defer func() {
		if err == nil {
			return
		}

		_ = ws.Close()
	}()

	for _, d := range strings.Split(dst, ",") {
		if d == "" {
			continue
		}

		u, err := ParseURL(d)
		if err != nil {
			return nil, errors.Wrap(err, "parse %v", d)
		}

		tlog.V("writer_url").Printw(d, "scheme", u.Scheme, "host", u.Host, "path", u.Path, "query", u.RawQuery, "from", loc.Caller(1))

		w, err := openw(u)
		if err != nil {
			return nil, errors.Wrap(err, "%v", d)
		}

		ws = append(ws, w)
	}

	if len(ws) == 1 {
		var ok bool
		if wc, ok = ws[0].(io.WriteCloser); ok {
			return wc, nil
		}
	}

	return ws, nil
}

func openw(u *url.URL) (io.Writer, error) {
	//	fmt.Fprintf(os.Stderr, "openw %q\n", fname)

	w, c, err := openwc(u, u.Path)
	if err != nil {
		return nil, err
	}

	return writeCloser(w, c), nil
}

func openwc(u *url.URL, base string) (w io.Writer, c io.Closer, err error) {
	//	fmt.Fprintf(os.Stderr, "openwc %q %q\n", base, ext)

	// w := os.Create("file.json.ez")
	// w = tlz.NewEncoder(w)
	// w = convert.NewJSON(w)

	var wrap []writerWrapper

more:
	ext := filepath.Ext(base)
	base = strings.TrimSuffix(base, ext)

	switch ext {
	case ".tlog", ".tl":
	case ".tlogdump", ".tldump":
		wrap = append(wrap, func(w io.Writer, c io.Closer) (io.Writer, io.Closer, error) {
			w = tlwire.NewDumper(w)

			return w, c, nil
		})
	case ".log", "":
		wrap = append(wrap, func(w io.Writer, c io.Closer) (io.Writer, io.Closer, error) {
			ff := tlog.LstdFlags
			ff = updateConsoleFlags(ff, u.RawQuery)

			w = tlog.NewConsoleWriter(w, ff)

			return w, c, nil
		})
	case ".tlz", ".eazy", ".ez":
		wrap = append(wrap, func(w io.Writer, c io.Closer) (io.Writer, io.Closer, error) {
			if f, ok := w.(*rotated.File); ok {
				f.OpenFile = RotatedTLZFileOpener(f.OpenFile)

				return f, c, nil
			}

			w = eazy.NewWriter(w, eazy.MiB)

			return w, c, nil
		})

		if ext == ".eazy" || ext == ".ez" {
			goto more
		}
	case ".json":
		wrap = append(wrap, func(w io.Writer, c io.Closer) (io.Writer, io.Closer, error) {
			w = convert.NewJSON(w)

			return w, c, nil
		})
	case ".logfmt":
		wrap = append(wrap, func(w io.Writer, c io.Closer) (io.Writer, io.Closer, error) {
			w = convert.NewLogfmt(w)

			return w, c, nil
		})
	case ".html":
		wrap = append(wrap, func(w io.Writer, c io.Closer) (io.Writer, io.Closer, error) {
			wc := writeCloser(w, c)
			w = convert.NewWeb(wc)
			c, _ = w.(io.Closer)

			return w, c, nil
		})
	case ".eazydump", ".ezdump":
		wrap = append(wrap, func(w io.Writer, c io.Closer) (io.Writer, io.Closer, error) {
			w = eazy.NewDumper(w)
			w = eazy.NewWriter(w, eazy.MiB)

			return w, c, nil
		})
	default:
		return nil, nil, errors.New("unsupported format: %v", ext)
	}

	w, c, err = openwriter(u, base)
	if err != nil {
		return nil, nil, err
	}

	for _, wrap := range wrap {
		w, c, err = wrap(w, c)
		if err != nil {
			return
		}
	}

	return w, c, nil
}

func openwriter(u *url.URL, base string) (w io.Writer, c io.Closer, err error) {
	switch base {
	case "-", "stdout":
		return os.Stdout, nil, nil
	case "", "stderr":
		return os.Stderr, nil, nil
	case "discard":
		return io.Discard, nil, nil
	}

	var f interface{}

	if u.Scheme != "" {
		f, err = openwurl(u)
	} else {
		f, err = openwfile(u)
	}
	if err != nil {
		return nil, nil, err
	}

	w = f.(io.Writer)
	c, _ = f.(io.Closer)

	return w, c, nil
}

func openwfile(u *url.URL) (interface{}, error) {
	fname := u.Path

	of := os.O_APPEND | os.O_WRONLY | os.O_CREATE
	of = updateFileFlags(of, u.RawQuery)

	mode := os.FileMode(0o644)

	q := u.Query()

	if rotated.IsPattern(filepath.Base(fname)) || q.Get("rotated") != "" {
		f := rotated.Create(fname)
		f.Flags = of
		f.Mode = mode
		f.OpenFile = openFileWriter

		if v := q.Get("max_file_size"); v != "" {
			x, err := ParseBytes(v)
			if err == nil {
				f.MaxFileSize = x
			}
		}

		if v := q.Get("max_file_age"); v != "" {
			x, err := time.ParseDuration(v)
			if err == nil {
				f.MaxFileAge = x
			}
		}

		if v := q.Get("max_total_size"); v != "" {
			x, err := ParseBytes(v)
			if err == nil {
				f.MaxTotalSize = x
			}
		}

		if v := q.Get("max_total_age"); v != "" {
			x, err := time.ParseDuration(v)
			if err == nil {
				f.MaxTotalAge = x
			}
		}

		return f, nil
	}

	return OpenFileWriter(fname, of, mode)
}

func openwurl(u *url.URL) (f interface{}, err error) {
	if u.Scheme == "file" {
		return openwfile(u)
	}

	switch u.Scheme {
	case "unix", "unixgram":
	default:
		return nil, errors.New("unsupported scheme: %v", u.Scheme)
	}

	return tlio.NewReWriter(func(w io.Writer, err error) (io.Writer, error) {
		if c, ok := w.(io.Closer); ok {
			_ = c.Close()
		}

		return net.Dial(u.Scheme, u.Path)
	}), nil
}

func OpenReader(src string) (rc io.ReadCloser, err error) {
	u, err := ParseURL(src)
	if err != nil {
		return nil, errors.Wrap(err, "parse %v", src)
	}

	r, err := openr(u)
	if err != nil {
		return nil, err
	}

	var ok bool
	if rc, ok = r.(io.ReadCloser); ok {
		return rc, nil
	}

	return tlio.NopCloser{
		Reader: r,
	}, nil
}

func openr(u *url.URL) (io.Reader, error) {
	r, c, err := openrc(u, u.Path)
	if err != nil {
		return nil, err
	}

	if c == nil {
		if _, ok := r.(io.Closer); ok {
			return tlio.NopCloser{Reader: r}, nil
		}

		return r, nil
	}

	if r.(interface{}) == c.(interface{}) {
		return r, nil
	}

	return tlio.ReadCloser{
		Reader: r,
		Closer: c,
	}, nil
}

func openrc(u *url.URL, base string) (r io.Reader, c io.Closer, err error) {
	var wrap []readerWrapper

more:
	ext := filepath.Ext(base)
	base = strings.TrimSuffix(base, ext)

	switch ext {
	case ".tlog", ".tl", "":
	case ".tlz", ".eazy", ".ez":
		wrap = append(wrap, func(r io.Reader, c io.Closer) (io.Reader, io.Closer, error) {
			r = eazy.NewReader(r)

			return r, c, nil
		})

		if ext == ".eazy" || ext == ".ez" {
			goto more
		}
	default:
		return nil, nil, errors.New("unsupported format: %v", ext)
	}

	r, c, err = openreader(u, base)
	if err != nil {
		return nil, nil, err
	}

	for _, wrap := range wrap {
		r, c, err = wrap(r, c)
		if err != nil {
			return
		}
	}

	return r, c, nil
}

func openreader(u *url.URL, base string) (r io.Reader, c io.Closer, err error) {
	switch base {
	case "-", "", "stdin":
		return os.Stdin, nil, nil
	}

	var f interface{}

	if u.Scheme != "" {
		f, err = openrurl(u)
	} else {
		f, err = openrfile(u)
	}
	if err != nil {
		return nil, nil, err
	}

	if rc, ok := f.(tlio.ReadCloser); ok {
		return rc.Reader, rc.Closer, nil
	}

	r = f.(io.Reader)
	c, _ = f.(io.Closer)

	return r, c, nil
}

func openrfile(u *url.URL) (interface{}, error) {
	return OpenFileReader(u.Path, os.O_RDONLY, 0)
}

func openrurl(u *url.URL) (interface{}, error) {
	if u.Scheme == "file" {
		return openrfile(u)
	}

	switch u.Scheme { //nolint:gocritic
	default:
		return nil, errors.New("unsupported scheme: %v", u.Scheme)
	}
}

func updateFileFlags(of int, q string) int {
	for _, c := range q {
		if c == '0' {
			of |= os.O_TRUNC
		}
	}

	return of
}

func updateConsoleFlags(ff int, q string) int {
	for _, c := range q {
		switch c {
		case 'd':
			ff |= tlog.LdetFlags
		case 'm':
			ff |= tlog.Lmilliseconds
		case 'M':
			ff |= tlog.Lmicroseconds
		case 'n':
			ff |= tlog.Lfuncname
		case 'f':
			ff &^= tlog.Llongfile
			ff |= tlog.Lshortfile
		case 'F':
			ff &^= tlog.Lshortfile
			ff |= tlog.Llongfile
		case 'U':
			ff |= tlog.LUTC
		}
	}

	return ff
}

func OSOpenFile(name string, flags int, mode os.FileMode) (interface{}, error) {
	return os.OpenFile(name, flags, mode)
}

func OpenFileReReader(open FileOpener) FileOpener {
	return func(name string, flags int, mode os.FileMode) (interface{}, error) {
		f, err := open(name, flags, mode)
		if err != nil {
			return nil, err
		}

		rs := f.(tlio.ReadSeeker)

		r, err := tlio.NewReReader(rs)
		if err != nil {
			return nil, errors.Wrap(err, "open ReReader")
		}

		r.Hook = func(old, cur int64) {
			tlog.Printw("file truncated", "name", name, "old_len", old)
		}

		c, ok := f.(io.Closer)
		if !ok {
			return r, nil
		}

		return tlio.ReadCloser{
			Reader: r,
			Closer: c,
		}, nil
	}
}

func OpenFileDumpReader(open FileOpener) FileOpener {
	tr := tlog.Start("read dumper")

	return func(name string, flags int, mode os.FileMode) (interface{}, error) {
		f, err := open(name, flags, mode)
		if err != nil {
			return nil, err
		}

		r := f.(io.Reader)

		r = &tlio.DumpReader{
			Reader: r,
			Span:   tr.Spawn("open dump reader", "file", name),
		}

		return r, nil
	}
}

func RotatedTLZFileOpener(below rotated.FileOpener) rotated.FileOpener {
	return func(name string, flags int, mode os.FileMode) (io.Writer, error) {
		w, err := below(name, flags, mode)
		if err != nil {
			return nil, errors.Wrap(err, "")
		}

		w = eazy.NewWriter(w, eazy.MiB)

		return w, nil
	}
}

func openFileWriter(name string, flags int, mode os.FileMode) (io.Writer, error) {
	file, err := OpenFileWriter(name, flags, mode)
	if err != nil {
		return nil, err
	}

	return file.(io.Writer), nil
}

func ParseURL(d string) (u *url.URL, err error) {
	u, err = url.Parse(d)
	if err != nil {
		return nil, err
	}

	if u.Opaque != "" {
		return nil, errors.New("unexpected opaque url")
	}

	if (u.Scheme == "file" || u.Scheme == "unix" || u.Scheme == "unixgram") && u.Host != "" {
		u.Path = path.Join(u.Host, u.Path)
		u.Host = ""
	}

	return u, nil
}

func ParseBytes(s string) (int64, error) {
	base := 10
	neg := false

	for strings.HasPrefix(s, "-") {
		neg = !neg
		s = s[1:]
	}

	if strings.HasPrefix(s, "0x") {
		s = s[2:]
		base = 16
	}

	l := 0

	for l < len(s) && s[l] >= '0' && (s[l] <= '9' || base == 16 && (s[l] >= 'a' && s[l] <= 'f' || s[l] >= 'A' && s[l] <= 'F')) {
		l++
	}

	if l == 0 {
		return 0, errors.New("bad size")
	}

	var m int64

	switch strings.ToLower(s[l:]) {
	case "b", "":
		m = 1
	case "kb", "k":
		m = 1000
	case "kib", "ki":
		m = 1024
	case "mb", "m":
		m = 1e6
	case "mib", "mi":
		m = 1 << 20
	case "gb", "g":
		m = 1e9
	case "gib", "gi":
		m = 1 << 30
	case "tb", "t":
		m = 1e12
	case "tib", "ti":
		m = 1 << 40
	default:
		return 0, errors.New("unsupported suffix: %v", s[l:])
	}

	x, err := strconv.ParseInt(s[:l], base, 64)
	if err != nil {
		return 0, err
	}

	if neg {
		x = -x
	}

	return x * m, nil
}

func Describe(tr tlog.Span, x interface{}) {
	describe(tr, x, 0)
}

func describe(tr tlog.Span, x interface{}, d int) {
	switch x := x.(type) {
	case tlio.MultiWriter:
		tr.Printw("describe", "d", d, "typ", tlog.NextAsType, x)

		for _, w := range x {
			describe(tr, w, d+1)
		}
	case tlio.ReadCloser:
		tr.Printw("describe", "d", d, "typ", tlog.NextAsType, x)

		describe(tr, x.Reader, d+1)
		describe(tr, x.Closer, d+1)
	case *tlio.ReReader:
		tr.Printw("describe", "d", d, "typ", tlog.NextAsType, x)

		describe(tr, x.ReadSeeker, d+1)
	case *eazy.Reader:
		tr.Printw("describe", "d", d, "typ", tlog.NextAsType, x)

		describe(tr, x.Reader, d+1)
	case *eazy.Writer:
		tr.Printw("describe", "d", d, "typ", tlog.NextAsType, x)

		describe(tr, x.Writer, d+1)
	case *tlog.ConsoleWriter:
		tr.Printw("describe", "d", d, "typ", tlog.NextAsType, x)

		describe(tr, x.Writer, d+1)
	case *os.File:
		tr.Printw("describe", "d", d, "typ", tlog.NextAsType, x, "name", x.Name())

	case *net.UnixConn:
		f, err := x.File()

		tr.Printw("describe", "d", d, "typ", tlog.NextAsType, x, "get_file_err", err)

		describe(tr, f, d+1)
	default:
		tr.Printw("describe", "d", d, "typ", tlog.NextAsType, x)
	}
}

func writeCloser(w io.Writer, c io.Closer) io.Writer {
	if c == nil {
		if _, ok := w.(io.Closer); ok {
			return tlio.NopCloser{Writer: w}
		}

		return w
	}

	if w.(interface{}) == c.(interface{}) {
		return w
	}

	return tlio.WriteCloser{
		Writer: w,
		Closer: c,
	}
}
