package tlflag

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nikandfor/errors"

	"github.com/nikandfor/tlog"
)

var OpenFile = os.OpenFile

func updateFlags(ff, of int, s string) (_, _ int) {
	for _, c := range s {
		switch c {
		case '0':
			of |= os.O_TRUNC
		case 'd':
			ff = tlog.LdetFlags
		case 's', 'S':
			ff |= tlog.Lspans | tlog.Lmessagespan
		case 'n':
			ff |= tlog.Lfuncname
		case 'f':
			ff |= tlog.Lshortfile
		case 'F':
			ff &^= tlog.Lshortfile
			ff |= tlog.Llongfile
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

func ParseDestination(dst string) (ws []tlog.Writer, cl func() error, err error) {
	var toclose []io.Closer

	cl = func() (err error) {
		for _, f := range toclose {
			if e := f.Close(); err == nil {
				err = e
			}
		}

		return
	}

	defer func() {
		if err == nil {
			return
		}

		_ = cl()

		cl = nil
	}()

	for _, d := range strings.Split(dst, ",") {
		if d == "" {
			continue
		}

		var opts string
		ff := tlog.LstdFlags
		of := 0
		if p := strings.IndexByte(d, ':'); p != -1 {
			opts = d[p+1:]
			d = d[:p]

			ff, of = updateFlags(ff, of, opts)
		}

		ext := filepath.Ext(d)

		switch ext {
		case "", ".log", ".proto", ".json":
		default:
			err = errors.New("unsupported file type: %v", ext)
			return
		}

		var fw io.Writer

		if fn := strings.TrimSuffix(d, ext); fn == "stderr" || fn == "" {
			fw = tlog.Stderr
		} else {
			var f *os.File
			f, err = OpenFile(d, os.O_CREATE|os.O_WRONLY|of, 0644)
			if err != nil {
				return
			}

			toclose = append(toclose, f)

			fw = f
		}

		var w tlog.Writer
		switch ext {
		case "", ".log":
			cw := tlog.NewConsoleWriter(fw, ff)

			updateConsoleLoggerOptions(cw, opts)

			w = cw
		case ".proto":
			w = tlog.NewProtoWriter(fw)
		case ".json":
			w = tlog.NewJSONWriter(fw)
		}

		ws = append(ws, w)
	}

	return
}
