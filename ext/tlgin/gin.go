package tlgin

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"tlog.app/go/tlog"
	"tlog.app/go/tlog/ext/tlhttp"
	"tlog.app/go/tlog/low"
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

	xtr := c.GetHeader(tlhttp.TraceIDKey)
	if xtr != "" {
		trid, err = tlog.IDFromString(xtr)
	}

	tr := l.NewSpan(0, trid, "http_request", "client_ip", c.ClientIP(), "method", c.Request.Method, "path", c.Request.URL.Path)
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

	ctx := c.Request.Context()
	ctx = tlog.ContextWithSpan(ctx, tr)
	c.Request = c.Request.WithContext(ctx)

	c.Set("tlog.par", trid)

	c.Set("tlog.span", tr)

	c.Header(tlhttp.TraceIDKey, tr.ID.StringFull())

	c.Next()
}

func SpanFromContext(c *gin.Context) (tr tlog.Span) {
	i, _ := c.Get("tlog.span")
	tr, _ = i.(tlog.Span)

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
