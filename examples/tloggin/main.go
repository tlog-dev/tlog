// +build ignore

package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nikandfor/cli/flag"
	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/examples/tloggin"
)

var (
	listen = flag.String("listen,l", ":8000", "address to listen to")
	v      = flag.String("verbose,v", "", "tlog verbosity")
)

func main() {
	flag.Parse()

	tlog.SetFilter(*v)

	r := gin.New()

	r.Use(tloggin.Traces)
	r.Use(tloggin.Dumper) // must be after Traces

	v1 := r.Group("v1/")

	v1.Any("*path", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"path": c.Param("path")})
	})

	tlog.Fatalf("listen: %v", r.Run(*listen))
}
