package main

import (
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nikandfor/cli/flag"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/ext/tlgin"
	"github.com/nikandfor/tlog/ext/tlprometheus"
)

var (
	listen = flag.String("listen,l", ":8000", "address to listen to")
	v      = flag.String("verbose,v", "", "tlog verbosity")
	det    = flag.Bool("det", false, "detailed tlog")
)

func main() {
	flag.Parse()

	ff := tlog.LstdFlags
	if *det {
		ff = tlog.LdetFlags
	}

	w := tlog.NewConsoleWriter(tlog.Stderr, ff)

	tlog.DefaultLogger = tlog.New(w)
	l := tlog.New(w)

	pw := tlprometheus.New()
	l.AppendWriter(pw)

	tlog.SetFilter(*v)

	pw.Logger = tlog.DefaultLogger

	pm := prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Name:        "example_metric2_ms",
		Help:        "request milliseconds from mm:ss:000",
		Objectives:  map[float64]float64{0.1: 0.1, 0.5: 0.1, 0.9: 0.1, 0.95: 0.01, 0.99: 0.01, 1: 0.01},
		ConstLabels: prometheus.Labels{"const": "label"},
	}, []string{"path"})

	l.RegisterMetric("example_metric1_ms", "request milliseconds from mm:ss:000", "summary", tlog.Labels{"const=label"})

	prometheus.MustRegister(pm)
	prometheus.MustRegister(pw)

	r := gin.New()

	r.GET("/stack", func(c *gin.Context) {
		var buf [10000]byte

		runtime.Stack(buf[:], true)

		fmt.Fprintf(tlog.Stderr, "%s", buf[:])

		c.String(http.StatusOK, "ok\n")
	})

	r.Use(tlgin.Logger)

	v1 := r.Group("v1")

	v1.Use(func(c *gin.Context) {
		tr := l.Start()
		defer tr.Finish()

		pth := c.Request.URL.Path

		v := time.Duration(time.Now().UnixNano()) % time.Second / time.Millisecond

		tr.Observe("example_metric1_ms", float64(v), tlog.Labels{"path=" + pth})

		pm.WithLabelValues(pth).Observe(float64(v))
	})

	v1.Any("*path", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"path": c.Param("path")})
	})

	r.GET("/metrics", func(c *gin.Context) {
		pw.ServeHTTP(c.Writer, c.Request)
	})

	r.GET("/metrics_prometheus", func(c *gin.Context) {
		promhttp.Handler().ServeHTTP(c.Writer, c.Request)
	})

	tlog.Fatalf("listen: %v", r.Run(*listen))
}
