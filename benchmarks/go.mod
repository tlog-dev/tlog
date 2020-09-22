module github.com/nikandfor/tlog/benchmarks

go 1.15

replace github.com/nikandfor/tlog => ../

require (
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/nikandfor/tlog v0.6.1-0.20200922004922-5af7073e9e44
	github.com/opentracing/opentracing-go v1.2.0
	github.com/sirupsen/logrus v1.6.0
	github.com/uber/jaeger-client-go v2.25.0+incompatible
	github.com/uber/jaeger-lib v2.2.0+incompatible // indirect
	go.uber.org/zap v1.16.0
)
