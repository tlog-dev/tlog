package web

import (
	"bufio"
	"context"
	"embed"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nikandfor/errors"
	"github.com/nikandfor/hacked/hnet"
	"tlog.app/go/eazy"

	"tlog.app/go/tlog"
	"tlog.app/go/tlog/convert"
)

type (
	Agent interface {
		Query(ctx context.Context, w io.Writer, ts int64, q string) error
	}

	Server struct {
		Agent Agent
		FS    http.FileSystem
	}

	response struct {
		req *http.Request
		w   io.Writer
		h   http.Header

		once    sync.Once
		nl      bool
		written bool
	}

	Proto func(context.Context, net.Conn) error
)

var (
	//go:embed index.html
	//go:embed manifest.json
	//go:embed static
	static embed.FS
)

func New(a Agent) (*Server, error) {
	return &Server{
		Agent: a,
		FS:    http.FS(static),
	}, nil
}

func (s *Server) Serve(ctx context.Context, l net.Listener, proto Proto) (err error) {
	var wg sync.WaitGroup
	defer wg.Wait()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	for {
		c, err := hnet.Accept(ctx, l)
		if err != nil {
			return err
		}

		wg.Add(1)

		go func() {
			defer wg.Done()

			_ = proto(ctx, c)
		}()
	}
}

func (s *Server) HandleConn(ctx context.Context, c net.Conn) (err error) {
	defer func() {
		e := c.Close()
		if err == nil {
			err = errors.Wrap(e, "close conn")
		}
	}()

	c = hnet.NewStoppableConn(ctx, c)

	br := bufio.NewReader(c)

	req, err := http.ReadRequest(br)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return errors.Wrap(err, "read request")
	}

	ctx, cancel := context.WithCancel(ctx)

	go func() {
		defer cancel()

		_, _ = io.Copy(io.Discard, c)
	}()

	resp := &response{
		req: req,
		w:   c,
	}

	defer func() {
		if resp.written {
			return
		}

		if err == nil {
			resp.WriteHeader(http.StatusNotFound)
			return
		}

		resp.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintf(resp, "%v\n", err)
	}()

	err = s.HandleRequest(ctx, resp, req)
	if err != nil {
		return err
	}

	return nil // TODO: handle Content-Length
}

func (s *Server) HandleRequest(ctx context.Context, rw http.ResponseWriter, req *http.Request) (err error) {
	tr := tlog.SpanFromContext(ctx)
	p := req.URL.Path

	tr.Printw("request", "method", req.Method, "url", req.URL)

	switch {
	case strings.HasPrefix(p, "/v0/events"):
		ts := queryInt64(req.URL, "ts", time.Now().UnixNano())

		var qdata []byte

		qdata, err = io.ReadAll(req.Body)
		if err != nil {
			return errors.Wrap(err, "read query")
		}

		var w io.Writer = rw

		switch ext := pathExt(p); ext {
		case ".tl", ".tlog":
		case ".tlz":
			w = eazy.NewWriter(w, eazy.MiB, 2*1024)
		case ".json":
			w = convert.NewJSON(w)
		case ".logfmt":
			w = convert.NewLogfmt(w)
		case ".html":
			ww := convert.NewWeb(w)
			defer closeWrap(ww, "close Web", &err)

			w = ww
		default:
			return errors.New("unsupported ext: %v", ext)
		}

		err = s.Agent.Query(ctx, w, ts, string(qdata))
		if errors.Is(err, context.Canceled) {
			err = nil
		}

		return errors.Wrap(err, "process query")
	}

	http.FileServer(s.FS).ServeHTTP(rw, req)

	return nil
}

func (r *response) WriteHeader(code int) {
	r.once.Do(func() {
		fmt.Fprintf(r.w, "HTTP/%d.%d %03d %s\r\n", 1, 0, code, http.StatusText(code))

		for k, v := range r.h {
			fmt.Fprintf(r.w, "%s:", k)

			for _, v := range v {
				fmt.Fprintf(r.w, " %s", v)
			}

			fmt.Fprintf(r.w, "\r\n")
		}

		fmt.Fprintf(r.w, "\r\n")
	})
}

func (r *response) Header() http.Header {
	if r.h == nil {
		r.h = make(http.Header)
	}

	return r.h
}

func (r *response) Write(p []byte) (n int, err error) {
	r.WriteHeader(http.StatusOK)

	n, err = r.w.Write(p)

	if n < len(p) {
		r.nl = p[n] == '\n'
	}

	return
}

func closeWrap(c io.Closer, msg string, errp *error) {
	e := c.Close()
	if *errp == nil {
		*errp = errors.Wrap(e, msg)
	}
}

func pathExt(name string) string {
	last := len(name)

	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '/' {
			return ""
		}

		if name[i] != '.' {
			continue
		}

		switch name[i:last] {
		case ".tl", ".tlog", ".tlz", ".json", ".logfmt", ".html":
			return name[i:]
		default:
			return ""
		}
	}

	return ""
}

func queryInt64(u *url.URL, key string, def int64) int64 {
	val := u.Query().Get(key)

	ts, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return def
	}

	return ts
}
