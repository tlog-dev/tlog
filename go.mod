module github.com/nikandfor/tlog

go 1.16

require (
	github.com/PaesslerAG/gval v1.1.2
	github.com/fsnotify/fsnotify v1.4.9
	github.com/gin-gonic/gin v1.7.2
	github.com/julienschmidt/httprouter v1.3.0
	github.com/nikandfor/assert v0.0.0-20220304193615-0dc830fd5c42
	github.com/nikandfor/cli v0.0.0-20220304154846-1d4fc796fa78
	github.com/nikandfor/clickhouse v0.0.0-20211202144204-3cea427c9385
	github.com/nikandfor/errors v0.5.1-0.20220304154616-1df269730b0b
	github.com/nikandfor/graceful v0.0.0-20220306090801-66b1ebdf6a58
	github.com/nikandfor/loc v0.2.0
	github.com/nikandfor/quantile v0.0.0-20201109213849-4905a12df281
	github.com/prometheus/client_golang v1.11.0
	github.com/stretchr/testify v1.7.0
	go.etcd.io/bbolt v1.3.5
	golang.org/x/term v0.0.0-20201126162022-7de9c90e9dd1
)

//replace github.com/nikandfor/assert => ../assert

//replace github.com/nikandfor/cli => ../cli

//replace github.com/nikandfor/clickhouse => ../clickhouse

//replace github.com/nikandfor/graceful => ../graceful

//replace github.com/nikandfor/loc => ../loc
