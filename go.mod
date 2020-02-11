module github.com/nikandfor/tlog

go 1.12

require (
	github.com/gin-gonic/gin v1.5.0
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/protobuf v1.3.2
	github.com/mailru/easyjson v0.0.0-20190626092158-b2ccc519800e // indirect
	github.com/nikandfor/cli v0.0.0-20191110144133-cc2d6c00dcff
	github.com/nikandfor/json v0.0.0-20200211224126-de471ddb3ea9
	github.com/stretchr/testify v1.4.0
)

// replace github.com/nikandfor/cli => ../cli
// replace github.com/nikandfor/json => ../json
