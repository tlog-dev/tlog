package tlflag

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nikandfor/tlog/wire"
	"github.com/nikandfor/tlog/writer"

	"github.com/nikandfor/tlog"
)

var OpenFile = os.OpenFile

func UpdateFlags(ff, of int, s string) (_, _ int) {
	for _, c := range s {
		switch c {
		case '0':
			of |= os.O_TRUNC
		case 'd':
			ff |= writer.LdetFlags
		case 'm':
			ff |= writer.Lmilliseconds
		case 'M':
			ff |= writer.Lmicroseconds
		case 's', 'S':
			ff |= writer.Lspans | writer.Lmessagespan
		case 'n':
			ff |= writer.Lfuncname
		case 'f':
			ff |= writer.Lshortfile
		case 'F':
			ff &^= writer.Lshortfile
			ff |= writer.Llongfile
		case 'U':
			ff |= writer.LUTC
		}
	}

	return ff, of
}

func updateConsoleLoggerOptions(w *writer.Console, s string) {
	for _, c := range s {
		switch c { //nolint:gocritic
		case 'S':
			w.IDWidth = 2 * len(tlog.ID{})
		}
	}
}

func ParseDestination(dst string) (w io.Writer, err error) {
	var ws writer.Tee

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
		ff := writer.LstdFlags
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
			w = wire.NewDumper(fw)
		case "", ".log":
			cw := writer.NewConsole(fw, ff)

			updateConsoleLoggerOptions(cw, opts)

			w = cw
			//	case ".proto":
			//		w = tlog.NewProtoWriter(fw)
			//	case ".json":
			//		w = tlog.NewJSONWriter(fw)
		}

		if fc != nil {
			w = writer.WriteCloser{
				Writer: w,
				Closer: fc,
			}
		}

		ws = append(ws, w)
	}

	return ws, nil
}
