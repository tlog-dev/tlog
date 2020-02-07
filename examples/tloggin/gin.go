package tloggin

import (
	"fmt"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nikandfor/tlog"
)

func Traces(c *gin.Context) {
	var trid tlog.ID

	if xtr := c.GetHeader("X-Traceid"); xtr != "" {
		var err error
		trid, err = tlog.IDFromString(xtr)
		if err != nil {
			tlog.Printf("bad trace id: %v", err)
			trid = tlog.ID{}
		}
	}

	tr := tlog.SpawnOrStart(trid)
	defer tr.Finish()

	defer func() {
		if p := recover(); p != nil {
			s := debug.Stack()
			tr.Printf("panic: %v\n%s", p, s)
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("%v", p)})
		}

		tr.Printf("%-15v | %v | %3v | %13.3fs | %-8v %v", c.ClientIP(), trid, c.Writer.Status(), time.Since(tr.Started).Seconds(), c.Request.Method, c.Request.URL.Path)
	}()

	tr.V("begin").Printf("%-15v | %v | %-8v %v", c.ClientIP(), trid, c.Request.Method, c.Request.URL.Path)

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
