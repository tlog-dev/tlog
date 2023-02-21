package tlflag

import (
	"io"
	"os"
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

		var opts string
		if p := strings.IndexByte(d, ':'); p != -1 {
			opts = d[p+1:]
			d = d[:p]
		} else if p = strings.IndexByte(d, '+'); p != -1 {
			opts = d[p+1:]
			d = d[:p]
		}

		w, err := openw(d, opts)
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

func openw(fname, opts string) (io.Writer, error) {
	//	fmt.Fprintf(os.Stderr, "openw %q\n", fname)

	w, c, err := openwc(fname, fname, opts)
	if err != nil {
		return nil, err
	}

	if c == nil {
		if _, ok := w.(io.Closer); ok {
			return tlio.NopCloser{Writer: w}, nil
		}

		return w, nil
	}

	if w.(interface{}) == c.(interface{}) {
		return w, nil
	}

	return tlio.WriteCloser{
		Writer: w,
		Closer: c,
	}, nil
}

func openwc(fname, base, opts string, wrap ...func(io.Writer) (io.Writer, error)) (w io.Writer, c io.Closer, err error) {
	ext := filepath.Ext(base)
	base = strings.TrimSuffix(base, ext)

	//	fmt.Fprintf(os.Stderr, "openwc %q %q\n", base, ext)

	// w := os.Create("file.json.ez")
	// w = tlz.NewEncoder(w)
	// w = convert.NewJSON(w)

	switch ext {
	case ".tlog", ".tl", ".tlogdump", ".tldump", ".log", "":
	case ".tlz":
	case ".json", ".logfmt":
	case ".eazy", ".ez":
		wrap = append(wrap, func(w io.Writer) (io.Writer, error) {
			return tlz.NewEncoder(w, tlz.MiB), nil
		})

		return openwc(fname, base, opts, wrap...)
	case ".eazydump", ".ezdump":
	default:
		return nil, nil, errors.New("unsupported format: %v", ext)
	}

	w, c, err = openwriter(fname, base, opts)
	if err != nil {
		return nil, nil, err
	}

	for _, wrap := range wrap {
		w, err = wrap(w)
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
		ff = updateConsoleFlags(ff, opts)

		w = tlog.NewConsoleWriter(w, ff)
	case ".json":
		w = convert.NewJSON(w)
	case ".logfmt":
		w = convert.NewLogfmt(w)
	case ".tlogdump", ".tldump":
		w = tlwire.NewDumper(w)
	case ".eazydump", ".ezdump":
		w = tlz.NewDumper(w)
	default:
		panic(ext)
	}

	return w, c, nil
}

func openwriter(fname, base, opts string) (io.Writer, io.Closer, error) {
	switch base {
	case "-", "stdout":
		return os.Stdout, nil, nil
	case "", "stderr":
		return os.Stderr, nil, nil
	}

	f, err := openwfile(fname, opts)
	if err != nil {
		return nil, nil, err
	}

	w := f.(io.Writer)
	c, _ := f.(io.Closer)

	return w, c, nil
}

func openwfile(fname, opts string) (interface{}, error) {
	of := os.O_APPEND | os.O_WRONLY | os.O_CREATE
	of = updateFileFlags(of, opts)

	mode := os.FileMode(0644)

	return OpenFileWriter(fname, of, mode)
}

func OpenReader(src string) (rc io.ReadCloser, err error) {
	r, err := openr(src, "")
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

func openr(fname, opts string) (io.Reader, error) {
	r, c, err := openrc(fname, fname, opts)
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

func openrc(fname, base, opts string, wrap ...func(io.Reader) (io.Reader, error)) (r io.Reader, c io.Closer, err error) {
	ext := filepath.Ext(base)
	base = strings.TrimSuffix(base, ext)

	switch ext {
	case ".tlog", ".tl", "":
	case ".tlz":
	case ".eazy", ".ez":
		wrap = append(wrap, func(r io.Reader) (io.Reader, error) {
			return tlz.NewDecoder(r), nil
		})

		return openrc(fname, base, opts, wrap...)
	default:
		return nil, nil, errors.New("unsupported format: %v", ext)
	}

	r, c, err = openreader(fname, base, opts)
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

func openreader(fname, base, opts string) (io.Reader, io.Closer, error) {
	switch base {
	case "-", "", "stdin":
		return os.Stdin, nil, nil
	}

	f, err := openrfile(fname, opts)
	if err != nil {
		return nil, nil, err
	}

	r := f.(io.Reader)
	c, _ := f.(io.Closer)

	return r, c, nil
}

func openrfile(fname, opts string) (interface{}, error) {
	return OpenFileReader(fname, os.O_RDONLY, 0)
}

func updateFileFlags(of int, s string) int {
	for _, c := range s {
		if c == '0' {
			of |= os.O_TRUNC
		}
	}

	return of
}

func updateConsoleFlags(ff int, s string) int {
	for _, c := range s {
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
