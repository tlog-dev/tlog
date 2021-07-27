module github.com/nikandfor/tlog

go 1.15

require (
	github.com/ClickHouse/clickhouse-go v1.4.3
	github.com/fsnotify/fsnotify v1.4.9
	github.com/gin-gonic/gin v1.7.2
	github.com/kr/pretty v0.2.1 // indirect
	github.com/nikandfor/cli v0.0.0-20210105003942-afe14413f747
	github.com/nikandfor/errors v0.3.1-0.20201212142705-56fda2c0e8b3
	github.com/nikandfor/loc v0.1.0
	github.com/nikandfor/quantile v0.0.0-20201109213849-4905a12df281
	github.com/prometheus/client_golang v1.10.0
	github.com/stretchr/testify v1.6.1
	go.etcd.io/bbolt v1.3.5
	golang.org/x/crypto v0.0.0-20201208171446-5f87f3452ae9
	golang.org/x/sys v0.0.0-20210326220804-49726bf1d181 // indirect
	google.golang.org/protobuf v1.25.0 // indirect
)

//replace github.com/nikandfor/loc => ../loc
