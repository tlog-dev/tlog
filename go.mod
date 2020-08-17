module github.com/nikandfor/tlog

go 1.13

require (
	github.com/gin-gonic/gin v1.5.0
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/protobuf v1.4.1
	github.com/nikandfor/cli v0.0.0-20200325075312-052d5b29bac6
	github.com/nikandfor/errors v0.1.0
	github.com/nikandfor/json v0.0.0-20200211224126-de471ddb3ea9
	github.com/stretchr/testify v1.4.0
	google.golang.org/protobuf v1.25.0
)

// replace github.com/nikandfor/cli => ../cli
// replace github.com/nikandfor/json => ../json
// replace github.com/nikandfor/xrain => ../xrain
