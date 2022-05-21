package web

import (
	"embed"
	"io"
	"io/fs"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/nikandfor/errors"
	"github.com/nikandfor/loc"
	"github.com/nikandfor/tlog"
)

//go:embed index.gohtmltmpl
var embedfs embed.FS

var webfs fs.FS = embedfs

func init() {
	if q := os.Getenv("TLOG_WEB_FS"); q != "" {
		tlog.Printw("web fs", "dir", q)

		webfs = os.DirFS(q)
	}
}

type (
	Service struct {
		r io.ReadSeeker
	}
)

func New() *Service {
	return &Service{}
}

//func (s *Service) HandleHTTP(w http.ResponseWriter, req *http.Request) {}

func (s *Service) HandleIndex(c *gin.Context) {
	var err error

	defer func() {
		if err != nil {
			tlog.Printw("handle index", "err", err, "", loc.Caller(1))
		}
	}()

	data, err := fs.ReadFile(webfs, "index.gohtmltmpl")
	if err != nil {
		err = errors.Wrap(err, "read file")
		return
	}

	_, err = c.Writer.Write(data)
	if err != nil {
		err = errors.Wrap(err, "write")
		return
	}
}

func (s *Service) HandleQuery(c *gin.Context) {
}
