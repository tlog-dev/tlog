module github.com/nikandfor/tlog

go 1.15

require (
	github.com/bmizerany/assert v0.0.0-20160611221934-b7ed37b82869 // indirect
	github.com/getsentry/sentry-go v0.7.0 // indirect
	github.com/gin-gonic/gin v1.6.3
	github.com/nikandfor/cli v0.0.0-20201215223928-0133e17b9ee4
	github.com/nikandfor/errors v0.3.1-0.20201212142705-56fda2c0e8b3
	github.com/nikandfor/loc v0.0.0-20201209201630-39582039abc5
	github.com/nikandfor/xrain v0.0.0-20200921231627-f669ab2645f2 // indirect
	github.com/pierrec/lz4 v2.6.0+incompatible // indirect
	github.com/pkg/errors v0.9.1
	github.com/stretchr/testify v1.6.1
	golang.org/x/crypto v0.0.0-20201208171446-5f87f3452ae9
)

//replace github.com/nikandfor/loc => ../loc
