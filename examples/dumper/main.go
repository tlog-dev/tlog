package main

import (
	"context"
	"io"
	"strings"

	"github.com/nikandfor/loc"
	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/tlio"
)

func main() {
	cw := tlog.NewConsoleWriter(tlog.Stderr, tlog.LdetFlags)
	tlog.DefaultLogger = tlog.New(cw)

	cw.StringOnNewLineMinLen = 10

	q := "some file content or socket...\nor some other data you want to dump to logs"

	w := io.Discard // just for example

	_ = copyStream(context.Background(), w, strings.NewReader(q))

	//	tlog.Printw("msg", "obj", map[string]string{"key": q, "key2": q})
}

func copyStream(ctx context.Context, w io.Writer, r io.Reader) (err error) {
	tr := tlog.SpawnFromContextOrStart(ctx, "copy_stream")
	defer func() { tr.Finish("err", err, tlog.KeyCaller, loc.Caller(1)) }()

	// Here is the trick
	lw := tlio.WriterFunc(func(p []byte) (int, error) {
		tr.Printw("copied block", "len", tlog.NextAsHex, len(p), "block", p)

		return len(p), nil
	})

	w = io.MultiWriter(w, lw)

	r = tlio.NopCloser{Reader: r} // preserve 20 bytes buffer, prevent WriteTo fastpath

	_, err = io.CopyBuffer(w, r, make([]byte, 20))

	return err
}
