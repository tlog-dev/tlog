package tlgin

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/ext/tlhttp"
	"github.com/nikandfor/tlog/low"
)

func Tracer(c *gin.Context) {
	tracer(tlog.DefaultLogger, c)
}

func CustomTracer(l *tlog.Logger) func(*gin.Context) {
	return func(c *gin.Context) {
		tracer(l, c)
	}
}

func tracer(l *tlog.Logger, c *gin.Context) {
	var trid tlog.ID
	var err error

	xtr := c.GetHeader(tlhttp.XTraceIDKey)
	if xtr != "" {
		trid, err = tlog.IDFromString(xtr)
		if err != nil {
			trid = tlog.ID{}
		}
	}

	tr := l.SpawnOrStart(trid, "http_request", "client_ip", c.ClientIP(), "meth", c.Request.Method, "path", c.Request.URL.Path)
	defer func() {
		if p := recover(); p != nil {
			s := debug.Stack()

			tr.Printw("panic", "panic", p, "panic_type", tlog.FormatNext("%T"), p, "stack_trace", low.UnsafeBytesToString(s), tlog.KeyLogLevel, tlog.Error)

			c.Status(http.StatusInternalServerError)
		}

		tr.Finish("status_code", c.Writer.Status())
	}()

	if err != nil {
		tr.Printw("bad parent trace id", "id", xtr, "err", err)
	}

	c.Set("tlog.par", trid)

	c.Set("tlog.span", tr)
	c.Set("tlog.id", tr.ID)

	c.Header(tlhttp.XTraceIDKey, tr.ID.StringFull())

	c.Next()
}

func logger(c *gin.Context) {
	tr := SpanFromContext(c)

	if tr.If("begin") {
		tr.Printw("request", "client_ip", c.ClientIP(), "meth", c.Request.Method, "path", c.Request.URL.Path)
	}

	defer func() {
		if p := recover(); p != nil {
			s := debug.Stack()

			tr.Printw("panic", "panic", p, "stack_trace", low.UnsafeBytesToString(s), tlog.KeyLogLevel, tlog.Error)
		}

		tr.Printw("response", "status_code", c.Writer.Status())
	}()

	c.Next()
}

func SpanFromContext(c *gin.Context) (tr tlog.Span) {
	i, ok := c.Get("tlog.span")
	if !ok {
		return
	}

	tr, _ = i.(tlog.Span)

	return
}

func IDFromContext(c *gin.Context) (id tlog.ID) {
	i, ok := c.Get("tlog.id")
	if !ok {
		return
	}

	id, _ = i.(tlog.ID)

	return
}

func Dumper(c *gin.Context) {
	tr := SpanFromContext(c)

	if tr.If("rawbody,rawrequest") {
		data, err := ioutil.ReadAll(c.Request.Body)
		if err != nil {
			tr.Printw("read body", "err", err)
			return
		}

		err = c.Request.Body.Close()
		if err != nil {
			tr.Printw("close body", "err", err)
			return
		}

		c.Request.Body = ioutil.NopCloser(bytes.NewReader(data))

		tr.Printw("request", "len", len(data), "data", data)
	}

	var rw *respWriter

	if tr.If("rawbody,rawresponse") {
		rw = &respWriter{ResponseWriter: c.Writer}

		c.Writer = rw
	}

	c.Next()

	if tr.If("rawbody,rawresponse") {
		tr.Printw("response", "len", rw.cp.Len(), "data", rw.cp.Bytes())
	}
}

type respWriter struct {
	gin.ResponseWriter
	cp bytes.Buffer
}

func (w *respWriter) Write(p []byte) (int, error) {
	_, _ = w.cp.Write(p)
	return w.ResponseWriter.Write(p)
}
