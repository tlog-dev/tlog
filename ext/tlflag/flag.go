package tlflag

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nikandfor/errors"
	"github.com/nikandfor/tlog"

	"github.com/nikandfor/tlog/tlio"
	"github.com/nikandfor/tlog/tlwire"
	"github.com/nikandfor/tlog/tlz"
)

var (
	OpenFileWriter = func(name string, flags int, mode os.FileMode) (io.WriteCloser, error) {
		return os.OpenFile(name, flags, mode)
	}

	OpenFileReader = func(name string, flags int, mode os.FileMode) (io.ReadCloser, error) {
		return os.OpenFile(name, flags, mode)
	}
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

func openwc(fname, base, opts string) (w io.Writer, c io.Closer, err error) {
	ext := filepath.Ext(base)
	base = strings.TrimSuffix(base, ext)

	//	fmt.Fprintf(os.Stderr, "openwc %q %q\n", base, ext)

	switch ext {
	case ".tlog", ".tl", ".tlogdump", ".tldump", ".log", "":
	case ".tlz":
	case ".eazy", ".ez":
		w, c, err = openwc(fname, base, opts)
		if err != nil {
			return
		}

		blockSize := tlz.MiB

		w = tlz.NewEncoder(w, blockSize)

		return w, c, nil
	case ".eazydump", ".ezdump":
	default:
		return nil, nil, errors.New("unsupported format: %v", ext)
	}

	w, c, err = openwriter(fname, base, opts)
	if err != nil {
		return nil, nil, err
	}

	switch ext {
	case ".tlog", ".tl":
	case ".tlz":
		blockSize := tlz.MiB

		w = tlz.NewEncoder(w, blockSize)
	case ".tlogdump", ".tldump":
		w = tlwire.NewDumper(w)
	case ".log", "":
		ff := tlog.LstdFlags
		ff = updateConsoleFlags(ff, opts)

		w = tlog.NewConsoleWriter(w, ff)
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

	return f, f, nil
}

func openwfile(fname, opts string) (io.WriteCloser, error) {
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

func openrc(fname, base, opts string) (r io.Reader, c io.Closer, err error) {
	ext := filepath.Ext(base)
	base = strings.TrimSuffix(base, ext)

	switch ext {
	case ".tlog", ".tl", "":
	case ".tlz":
	case ".eazy", ".ez":
		r, c, err = openrc(fname, base, opts)
		if err != nil {
			return
		}

		r = tlz.NewDecoder(r)

		return r, c, nil
	default:
		return nil, nil, errors.New("unsupported format: %v", ext)
	}

	r, c, err = openreader(fname, base, opts)
	if err != nil {
		return nil, nil, err
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

	return f, f, nil
}

func openrfile(fname, opts string) (io.ReadCloser, error) {
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
