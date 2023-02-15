module github.com/nikandfor/tlog

go 1.16

require (
	github.com/fsnotify/fsnotify v1.4.9
	github.com/gin-gonic/gin v1.7.2
	github.com/julienschmidt/httprouter v1.3.0
	github.com/nikandfor/assert v0.0.0-20220310091831-57b3fdb27159
	github.com/nikandfor/cli v0.0.0-20220414143810-caca4acd49c1
	github.com/nikandfor/clickhouse v0.0.0-20211202144204-3cea427c9385
	github.com/nikandfor/errors v0.6.0
	github.com/nikandfor/graceful v0.0.0-20220310092206-1de14c7618d9
	github.com/nikandfor/loc v0.3.0
	github.com/nikandfor/quantile v0.0.0-20201109213849-4905a12df281
	github.com/prometheus/client_golang v1.11.1
	github.com/stretchr/testify v1.7.0
	go.etcd.io/bbolt v1.3.5
	golang.org/x/term v0.0.0-20201126162022-7de9c90e9dd1
)

//replace github.com/nikandfor/assert => ../assert

//replace github.com/nikandfor/cli => ../cli

//replace github.com/nikandfor/clickhouse => ../clickhouse

//replace github.com/nikandfor/graceful => ../graceful

//replace github.com/nikandfor/loc => ../loc
