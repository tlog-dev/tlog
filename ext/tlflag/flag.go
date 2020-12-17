package tlflag

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nikandfor/errors"
	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/compress"
)

type (
	nopCloser struct {
		io.Reader
		io.Writer
	}

	readCloser struct {
		io.Reader
		io.Closer
	}
)

var OpenFile = os.OpenFile

func UpdateFlags(ff, of int, s string) (_, _ int) {
	for _, c := range s {
		switch c {
		case '0':
			of |= os.O_TRUNC
		case 'd':
			ff |= tlog.LdetFlags
		case 'm':
			ff |= tlog.Lmilliseconds
		case 'M':
			ff |= tlog.Lmicroseconds
		case 'n':
			ff |= tlog.Lfuncname
		case 'f':
			ff |= tlog.Lshortfile
		case 'F':
			ff &^= tlog.Lshortfile
			ff |= tlog.Llongfile
		case 'U':
			ff |= tlog.LUTC
		}
	}

	return ff, of
}

func updateConsoleLoggerOptions(w *tlog.ConsoleWriter, s string) {
	for _, c := range s {
		switch c { //nolint:gocritic
		case 'S':
			w.IDWidth = 2 * len(tlog.ID{})
		}
	}
}

func OpenWriter(dst string) (w io.WriteCloser, err error) {
	var ws tlog.TeeWriter

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
		ff := tlog.LstdFlags
		of := os.O_APPEND | os.O_WRONLY | os.O_CREATE
		if p := strings.IndexByte(d, ':'); p != -1 {
			opts = d[p+1:]
			d = d[:p]

			ff, of = UpdateFlags(ff, of, opts)
		}

		w, err = openw(d, d, ff, of, 0644)
		if err != nil {
			return nil, errors.Wrap(err, "%v", d)
		}

		ws = append(ws, w)
	}

	if len(ws) == 1 {
		var ok bool
		if w, ok = ws[0].(io.WriteCloser); ok {
			return w, nil
		}

		return nopCloser{
			Writer: ws[0],
		}, nil
	}

	return ws, nil
}

func openw(fn, fmt string, ff, of int, mode os.FileMode) (w io.WriteCloser, err error) {
	ext := filepath.Ext(fmt)

	switch ext {
	case ".tlog", ".tl", ".dump", ".log", "":
		switch strings.TrimSuffix(fmt, ext) {
		case "", "stderr":
			w = nopCloser{Writer: os.Stderr}
		case "-", "stdout":
			w = nopCloser{Writer: os.Stdout}
		default:
			w, err = OpenFile(fn, of, mode)
		}
	case ".ez":
		w, err = openw(fn, strings.TrimSuffix(fmt, ext), ff, of, mode)
	default:
		err = errors.New("unsupported file ext: %v", ext)
	}

	if err != nil {
		return
	}

	cl, _ := w.(io.Closer)

	var ww io.Writer
	switch ext {
	case ".tlog", ".tl":
	case ".dump":
		ww = tlog.NewDumper(w)
	case ".ez":
		ww = compress.NewEncoder(w, 1<<20)
	case ".log", "":
		ww = tlog.NewConsoleWriter(w, ff)
	}

	if ww != nil {
		w = tlog.WriteCloser{
			Writer: ww,
			Closer: cl,
		}
	}

	return w, nil
}

func OpenReader(src string) (r io.ReadCloser, err error) {
	return openr(src, src)
}

func openr(fn, fmt string) (r io.ReadCloser, err error) {
	ext := filepath.Ext(fmt)

	switch ext {
	case ".tlog", ".tl", "":
		switch strings.TrimSuffix(fmt, ext) {
		case "", "-", "stdin":
			r = nopCloser{Reader: os.Stdin}
		default:
			r, err = OpenFile(fn, os.O_RDONLY, 0)
		}
	case ".ez":
		r, err = openr(fn, strings.TrimSuffix(fmt, ext))
	default:
		err = errors.New("unsupported file ext: %v", ext)
	}

	if err != nil {
		return
	}

	cl, _ := r.(io.Closer)

	var rr io.Reader
	switch ext {
	case ".tlog", ".tl", "":
	case ".ez":
		rr = compress.NewDecoder(r)
	}

	if rr != nil {
		r = readCloser{
			Reader: rr,
			Closer: cl,
		}
	}

	return
}

func (nopCloser) Close() error { return nil }

func (c nopCloser) Fd() uintptr {
	if c.Writer == nil {
		return 1<<64 - 1
	}

	switch f := c.Writer.(type) {
	case interface {
		Fd() uintptr
	}:
		return f.Fd()
	case interface {
		Fd() int
	}:
		return uintptr(f.Fd())
	}

	return 1<<64 - 1
}
