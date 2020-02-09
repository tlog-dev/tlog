package tlog

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLocationStackTraceFill(t *testing.T) {
	st := make(Trace, 1)

	st = StackTraceFill(0, st)

	assert.Len(t, st, 1)
	assert.Equal(t, "location_stacktrace_test.go:13", st[0].String())
}

func testStackTraceInside() (st Trace) {
	func() {
		func() {
			st = StackTrace(1, 3)
		}()
	}()
	return
}

func TestLocationStackTrace(t *testing.T) {
	var st Trace
	func() {
		func() {
			st = testStackTraceInside()
		}()
	}()

	assert.Len(t, st, 3)
	assert.Equal(t, "location_stacktrace_test.go:23", st[0].String())
	assert.Equal(t, "location_stacktrace_test.go:24", st[1].String())
	assert.Equal(t, "location_stacktrace_test.go:32", st[2].String())

	re := `tlog.testStackTraceInside.func1                               at [\w.-/]*location_stacktrace_test.go:23
tlog.testStackTraceInside                                     at [\w.-/]*location_stacktrace_test.go:24
tlog.TestLocationStackTrace.func1.1                           at [\w.-/]*location_stacktrace_test.go:32
`
	ok, err := regexp.MatchString(re, st.String())
	assert.NoError(t, err)
	assert.True(t, ok, "expected:\n%v\ngot:\n%v\n", re, st.String())
}
