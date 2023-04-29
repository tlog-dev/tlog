package tlflag

import (
	"io"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/nikandfor/errors"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/convert"
	"github.com/nikandfor/tlog/tlio"
	"github.com/nikandfor/tlog/tlwire"
	"github.com/nikandfor/tlog/tlz"
)

type (
	FileOpener func(name string, flags int, mode os.FileMode) (interface{}, error)
)

var (
	OpenFileWriter = OSOpenFile
	OpenFileReader = OpenFileReReader(OSOpenFile)
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

		// tlog.Printw(d, "scheme", u.Scheme, "host", u.Host, "path", u.Path, "query", u.RawQuery, "from", loc.Caller(1))

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

func openwc(u *url.URL, base string, wrap ...func(io.Writer, io.Closer) (io.Writer, io.Closer, error)) (w io.Writer, c io.Closer, err error) {
	ext := filepath.Ext(base)
	base = strings.TrimSuffix(base, ext)

	//	fmt.Fprintf(os.Stderr, "openwc %q %q\n", base, ext)

	// w := os.Create("file.json.ez")
	// w = tlz.NewEncoder(w)
	// w = convert.NewJSON(w)

	switch ext {
	case ".tlog", ".tl", ".tlogdump", ".tldump", ".log", "":
	case ".tlz":
	case ".json", ".logfmt", ".html":
	case ".eazy", ".ez":
		wrap = append(wrap, func(w io.Writer, c io.Closer) (io.Writer, io.Closer, error) {
			w = tlz.NewEncoder(w, tlz.MiB)

			return w, c, nil
		})

		return openwc(u, base, wrap...)
	case ".eazydump", ".ezdump":
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

	switch ext {
	case ".tlog", ".tl":
	case ".tlz":
		blockSize := tlz.MiB

		w = tlz.NewEncoder(w, blockSize)
	case ".log", "":
		ff := tlog.LstdFlags
		ff = updateConsoleFlags(ff, u.RawQuery)

		w = tlog.NewConsoleWriter(w, ff)
	case ".json":
		w = convert.NewJSON(w)
	case ".logfmt":
		w = convert.NewLogfmt(w)
	case ".html":
		wc := writeCloser(w, c)
		w = convert.NewWeb(wc)
		c, _ = w.(io.Closer)
	case ".tlogdump", ".tldump":
		w = tlwire.NewDumper(w)
	case ".eazydump", ".ezdump":
		w = tlz.NewDumper(w)
	default:
		panic(ext)
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

func openrc(u *url.URL, base string, wrap ...func(io.Reader) (io.Reader, error)) (r io.Reader, c io.Closer, err error) {
	ext := filepath.Ext(base)
	base = strings.TrimSuffix(base, ext)

	switch ext {
	case ".tlog", ".tl", "":
	case ".tlz":
	case ".eazy", ".ez":
		wrap = append(wrap, func(r io.Reader) (io.Reader, error) {
			return tlz.NewDecoder(r), nil
		})

		return openrc(u, base, wrap...)
	default:
		return nil, nil, errors.New("unsupported format: %v", ext)
	}

	r, c, err = openreader(u, base)
	if err != nil {
		return nil, nil, err
	}

	for _, wrap := range wrap {
		r, err = wrap(r)
		if err != nil {
			return
		}
	}

	switch ext {
	case ".tlog", ".tl", "":
	case ".tlz":
		r = tlz.NewDecoder(r)
	default:
		panic(ext)
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

func ParseURL(d string) (u *url.URL, err error) {
	u, err = url.Parse(d)
	if err != nil {
		return nil, err
	}

	if (u.Scheme == "file" || u.Scheme == "unix" || u.Scheme == "unixgram") && u.Host != "" {
		u.Path = path.Join(u.Host, u.Path)
		u.Host = ""
	}

	return u, nil
}

func DumpWriter(tr tlog.Span, w io.Writer) {
	dumpWriter(tr, w, 0)
}

func dumpWriter(tr tlog.Span, w io.Writer, d int) {
	switch w := w.(type) {
	case tlio.MultiWriter:
		tr.Printw("writer", "d", d, "typ", tlog.NextAsType, w)

		for _, w := range w {
			dumpWriter(tr, w, d+1)
		}
	case *tlog.ConsoleWriter:
		tr.Printw("writer", "d", d, "typ", tlog.NextAsType, w)

		dumpWriter(tr, w.Writer, d+1)
	case *os.File:
		tr.Printw("writer", "d", d, "typ", tlog.NextAsType, w, "name", w.Name())

	case *net.UnixConn:
		f, err := w.File()

		tr.Printw("writer", "d", d, "typ", tlog.NextAsType, w, "get_file_err", err)

		dumpWriter(tr, f, d+1)

	default:
		tr.Printw("writer", "d", d, "typ", tlog.NextAsType, w)
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
