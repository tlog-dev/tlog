module github.com/nikandfor/tlog

go 1.13

require (
	github.com/bmizerany/assert v0.0.0-20160611221934-b7ed37b82869
	github.com/getsentry/sentry-go v0.7.0
	github.com/gin-gonic/gin v1.5.0
	github.com/golang/protobuf v1.4.2
	github.com/nikandfor/cli v0.0.0-20200325075312-052d5b29bac6
	github.com/nikandfor/errors v0.3.1-0.20201013001757-3b7d42817e44
	github.com/nikandfor/json v0.2.0
	github.com/nikandfor/quantile v0.0.0-20200824213034-5a47c65eb02b
	github.com/nikandfor/xrain v0.0.0-20200921231627-f669ab2645f2
	github.com/pierrec/lz4 v2.6.0+incompatible // indirect
	github.com/pkg/errors v0.8.1
	github.com/prometheus/client_golang v1.7.1
	github.com/prometheus/client_model v0.2.0
	github.com/stretchr/testify v1.6.1
	go.opentelemetry.io/otel v0.11.0
	golang.org/x/crypto v0.0.0-20201012173705-84dcc777aaee
	google.golang.org/protobuf v1.25.0
	gopkg.in/fsnotify.v1 v1.4.7
)

// replace github.com/nikandfor/cli => ../cli
// replace github.com/nikandfor/json => ../json
// replace github.com/nikandfor/quantile => ../quantile
// replace github.com/nikandfor/xrain => ../xrain
