package tlgin

import (
	"bytes"
	"io/ioutil"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/nikandfor/tlog"
)

func Tracer(c *gin.Context) {
	tracer(tlog.DefaultLogger, c)
}

func CustomTracer(l *tlog.Logger) func(*gin.Context) {
	return func(c *gin.Context) {
		tracer(l, c)
	}
}

func Logger(c *gin.Context) {
	logger(c, true)
}

func tracer(l *tlog.Logger, c *gin.Context) {
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

	c.Set("tlog.par", trid)

	c.Set("tlog.span", tr)
	c.Set("tlog.id", tr.ID)

	c.Header("X-Traceid", tr.ID.FullString())

	c.Next()
}

func logger(c *gin.Context, ptid bool) {
	tr := SpanFromContext(c)

	par, _ := c.Value("tlog.par").(tlog.ID)

	if tr := tr.V("begin"); tr.Valid() {
		if ptid {
			tr.Printf("%-15v | %v | %-8v %v", c.ClientIP(), par, c.Request.Method, c.Request.URL.Path)
		} else {
			tr.Printf("%-15v | %-8v %v", c.ClientIP(), c.Request.Method, c.Request.URL.Path)
		}
	}

	defer func() {
		if p := recover(); p != nil {
			s := debug.Stack()

			tr.Printf("panic: %v\n%s", p, s)
			//	c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("%v", p)})
		}

		if ptid {
			tr.Printf("%-15v | %v | %3v | %13.3fs | %-8v %v",
				c.ClientIP(), par, c.Writer.Status(), time.Since(tr.StartedAt).Seconds(), c.Request.Method, c.Request.URL.Path)
		} else {
			tr.Printf("%-15v | %3v | %13.3fs | %-8v %v",
				c.ClientIP(), c.Writer.Status(), time.Since(tr.StartedAt).Seconds(), c.Request.Method, c.Request.URL.Path)
		}
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
