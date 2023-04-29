package web

import (
	"bufio"
	"context"
	"embed"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"sync"

	"github.com/nikandfor/errors"
	"github.com/nikandfor/hacked/hnet"
)

type (
	Agent interface{}

	Server struct {
		t *template.Template
		a Agent
	}

	response struct {
		req *http.Request
		w   io.Writer
		h   http.Header

		once sync.Once
		nl   bool
	}

	Proto func(context.Context, net.Conn) error
)

//go:embed *.tmpl
var embedded embed.FS

func New(a Agent) (*Server, error) {
	t := template.New("main")
	t, err := t.ParseFS(embedded, "*.tmpl")
	if err != nil {
		return nil, errors.Wrap(err, "load templates")
	}

	return &Server{
		t: t,
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

	resp := &response{
		req: req,
		w:   c,
	}

	err = s.HandleRequest(ctx, resp, req)
	if err != nil {
		return err
	}

	return nil // TODO: handle Content-Length
}

func (s *Server) HandleRequest(ctx context.Context, w io.Writer, req *http.Request) error {
	err := s.t.Execute(w, "hello")
	if err != nil {
		return errors.Wrap(err, "exec template")
	}

	return nil
}

func (r *response) WriteHeader(code int) {
	r.once.Do(func() {
		fmt.Fprintf(r.w, "HTTP/%d.%d %03d %s\r\n", r.req.ProtoMajor, r.req.ProtoMinor, code, http.StatusText(code))

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
