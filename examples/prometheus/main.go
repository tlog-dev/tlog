package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nikandfor/cli/flag"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/ext/tlflag"
	"github.com/nikandfor/tlog/ext/tlgin"
	"github.com/nikandfor/tlog/ext/tlprometheus"
	"github.com/nikandfor/tlog/tlio"
)

var (
	listen = flag.String("listen,l", ":8000", "address to listen to")
	log    = flag.String("log", "stderr+dm", "log file")
	v      = flag.String("verbose,v", "", "tlog verbosity")
)

func main() {
	flag.Parse()

	w, err := tlflag.OpenWriter(*log)
	if err != nil {
		tlog.Printw("open log file", "err", err)

		return
	}

	pw := tlprometheus.New()

	w = tlio.NewTeeWriter(w, pw)

	l := tlog.New(w)

	tlog.DefaultLogger = tlog.New(tlog.NewConsoleWriter(tlog.Stderr, tlog.LdetFlags|tlog.Lmilliseconds))

	//	l.SetLabels(tlog.FillLabelsWithDefaults("_hostname", "_pid"))
	l.SetLabels(tlog.Labels{"global=label"})

	l.SetFilter(*v)

	pm := prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Name:        "fully_qualified_metric_name_2_with_units",
		Help:        "help message that describes metric",
		Objectives:  map[float64]float64{0.1: 0.1, 0.5: 0.1, 0.9: 0.1, 0.95: 0.01, 0.99: 0.01, 1: 0.01},
		ConstLabels: prometheus.Labels{"const": "label"},
	}, []string{"path"})

	l.RegisterMetric("fully_qualified_metric_name_with_units", tlog.MetricSummary, "help message that describes metric",
		"quantile", []float64{0.1, 0.5, 0.9, 0.95, 0.99, 1},
		tlog.KeyLabels, tlog.Labels{"const=label"})

	prometheus.MustRegister(pm)
	//	prometheus.MustRegister(pw)

	r := gin.New()

	r.Use(tlgin.CustomTracer(l))

	v1 := r.Group("v1")

	v1.Use(func(c *gin.Context) {
		tr := tlgin.SpanFromContext(c)

		pth := c.Request.URL.Path

		tr.Observe("fully_qualified_metric_name_with_units", 123.456, "path", pth)

		pm.WithLabelValues(pth).Observe(float64(123.456))
	})

	v1.GET("*path", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"path": c.Param("path")})
	})

	r.GET("/metrics", func(c *gin.Context) {
		pw.ServeHTTP(c.Writer, c.Request)
	})

	r.GET("/metrics_prometheus", func(c *gin.Context) {
		promhttp.Handler().ServeHTTP(c.Writer, c.Request)
	})

	err = r.Run(*listen)
	tlog.Printw("listen", "err", err)
}
