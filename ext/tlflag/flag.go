package tlflag

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nikandfor/tlog"
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

func ParseDestination(dst string) (w io.Writer, err error) {
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
		of := os.O_APPEND
		if p := strings.IndexByte(d, ':'); p != -1 {
			opts = d[p+1:]
			d = d[:p]

			ff, of = UpdateFlags(ff, of, opts)
		}

		ext := filepath.Ext(d)

		switch ext {
		case "", ".log", ".tl", ".tlog", ".dump": //, ".proto", ".json":
		default:
			err = fmt.Errorf("unsupported file type: %v", ext)
			return
		}

		var fw io.Writer
		var fc io.Closer

		if fn := strings.TrimSuffix(d, ext); fn == "stderr" || fn == "-" || fn == "" {
			fw = tlog.Stderr
		} else {
			var f *os.File
			f, err = OpenFile(d, os.O_CREATE|os.O_WRONLY|of, 0644)
			if err != nil {
				return
			}

			fw = f
			fc = f
		}

		var w io.Writer
		switch ext {
		case ".tl", ".tlog":
			w = fw
		case ".dump":
			w = tlog.NewDumper(fw)
		case "", ".log":
			cw := tlog.NewConsoleWriter(fw, ff)

			updateConsoleLoggerOptions(cw, opts)

			w = cw
			//	case ".proto":
			//		w = tlog.NewProtoWriter(fw)
			//	case ".json":
			//		w = tlog.NewJSONWriter(fw)
		}

		if fc != nil {
			w = tlog.WriteCloser{
				Writer: w,
				Closer: fc,
			}
		}

		ws = append(ws, w)
	}

	if len(ws) == 1 {
		return ws[0], nil
	}

	return ws, nil
}
