module github.com/nikandfor/tlog

go 1.16

require (
	github.com/PaesslerAG/gval v1.1.2 // indirect
	github.com/fsnotify/fsnotify v1.4.9
	github.com/gin-gonic/gin v1.7.2
	github.com/nikandfor/cli v0.0.0-20210105003942-afe14413f747
	github.com/nikandfor/clickhouse v0.0.0-20211123135434-f570acb7f019
	github.com/nikandfor/errors v0.4.0
	github.com/nikandfor/graceful v0.0.0-20211115215916-d1e69cb51d77
	github.com/nikandfor/loc v0.1.1-0.20210914135013-829520244234
	github.com/nikandfor/quantile v0.0.0-20201109213849-4905a12df281
	github.com/prometheus/client_golang v1.11.0
	github.com/stretchr/testify v1.7.0
	go.etcd.io/bbolt v1.3.5
	golang.org/x/term v0.0.0-20201126162022-7de9c90e9dd1
)

//replace github.com/nikandfor/loc => ../loc

replace github.com/nikandfor/graceful => ../graceful

replace github.com/nikandfor/clickhouse => ../clickhouse
