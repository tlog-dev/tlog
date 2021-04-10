module github.com/nikandfor/tlog

go 1.15

require (
	github.com/ClickHouse/clickhouse-go v1.4.3
	github.com/fsnotify/fsnotify v1.4.9
	github.com/gin-gonic/gin v1.6.3
	github.com/golang/protobuf v1.4.2 // indirect
	github.com/google/go-cmp v0.5.4 // indirect
	github.com/json-iterator/go v1.1.10 // indirect
	github.com/kr/pretty v0.2.1 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.1 // indirect
	github.com/nikandfor/cli v0.0.0-20210105003942-afe14413f747
	github.com/nikandfor/errors v0.3.1-0.20201212142705-56fda2c0e8b3
	github.com/nikandfor/loc v0.0.0-20201209201630-39582039abc5
	github.com/stretchr/testify v1.6.1
	go.etcd.io/bbolt v1.3.5
	golang.org/x/crypto v0.0.0-20201208171446-5f87f3452ae9
	golang.org/x/sys v0.0.0-20210326220804-49726bf1d181 // indirect
	google.golang.org/protobuf v1.25.0 // indirect
)

//replace github.com/nikandfor/loc => ../loc
