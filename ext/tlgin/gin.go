package tlgin

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/nikandfor/tlog"
)

func Tracer(c *gin.Context) {
	traces(tlog.DefaultLogger, c, true)
}

func Logger(c *gin.Context) {
	traces(tlog.DefaultLogger, c, false)
}

func CustomTracer(l *tlog.Logger) func(*gin.Context) {
	return func(c *gin.Context) {
		traces(l, c, true)
	}
}

func CustomLogger(l *tlog.Logger) func(*gin.Context) {
	return func(c *gin.Context) {
		traces(l, c, false)
	}
}

func traces(l *tlog.Logger, c *gin.Context, ptid bool) {
	var trid tlog.ID
	var err error

	xtr := c.GetHeader("X-Traceid")
	if xtr != "" {
		trid, err = tlog.IDFromString(xtr)
		if err != nil {
			trid = tlog.ID{}
		}
	}

	tr := l.SpawnOrStart(trid)
	defer tr.Finish()

	if err != nil {
		tr.Printf("bad parent trace id %v: %v", xtr, err)
	}

	defer func() {
		if p := recover(); p != nil {
			s := debug.Stack()
			tr.Printf("panic: %v\n%s", p, s)
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("%v", p)})
		}

		if ptid {
			tr.Printf("%-15v | %v | %3v | %13.3fs | %-8v %v",
				c.ClientIP(), trid, c.Writer.Status(), time.Since(time.Unix(0, tr.Started)).Seconds(), c.Request.Method, c.Request.URL.Path)
		} else {
			tr.Printf("%-15v | %3v | %13.3fs | %-8v %v",
				c.ClientIP(), c.Writer.Status(), time.Since(time.Unix(0, tr.Started)).Seconds(), c.Request.Method, c.Request.URL.Path)
		}
	}()

	if tr := tr.V("begin"); tr.Valid() {
		if ptid {
			tr.Printf("%-15v | %v | %-8v %v", c.ClientIP(), trid, c.Request.Method, c.Request.URL.Path)
		} else {
			tr.Printf("%-15v | %-8v %v", c.ClientIP(), c.Request.Method, c.Request.URL.Path)
		}
	}

	c.Set("trace", tr)
	c.Set("traceid", tr.ID)

	c.Header("X-Traceid", tr.ID.FullString())

	c.Next()
}

func TraceFromContext(c *gin.Context) (tr tlog.Span) {
	i, ok := c.Get("trace")
	if !ok {
		return
	}
	tr, _ = i.(tlog.Span)
	return
}

func TraceIDFromContext(c *gin.Context) (id tlog.ID) {
	i, ok := c.Get("traceid")
	if !ok {
		return
	}
	id, _ = i.(tlog.ID)
	return
}

func Dumper(c *gin.Context) {
	tr := TraceFromContext(c)

	if tr := tr.V("rawbody,rawrequest"); tr.Valid() {
		data, err := ioutil.ReadAll(c.Request.Body)
		if err != nil {
			tr.Printf("read body: %v", err)
			return
		}

		err = c.Request.Body.Close()
		if err != nil {
			tr.Printf("close body: %v", err)
			return
		}

		c.Request.Body = ioutil.NopCloser(bytes.NewReader(data))

		tr.Printf("request:  len %5d  %-8v %v\n%s", len(data), c.Request.Method, c.Request.URL.Path, data)
	}

	var rw *respWriter

	if tr := tr.V("rawbody,rawresponse"); tr.Valid() {
		rw = &respWriter{ResponseWriter: c.Writer}

		c.Writer = rw
	}

	c.Next()

	if tr := tr.V("rawbody,rawresponse"); tr.Valid() {
		tr.Printf("response: len %5d  %-8v %v  => %3d\n%s", rw.cp.Len(), c.Request.Method, c.Request.URL.Path, c.Writer.Status(), rw.cp.Bytes())
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
