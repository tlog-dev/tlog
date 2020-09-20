package tlog

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLocationStackTraceFill(t *testing.T) {
	st := make(StackTrace, 1)

	st = FillCallers(0, st)

	assert.Len(t, st, 1)
	assert.Equal(t, "location_stacktrace_test.go:14", st[0].String())
}

func testStackTraceInside() (st StackTrace) {
	func() {
		func() {
			st = Callers(1, 3)
		}()
	}()
	return
}

func TestLocationStackTraceString(t *testing.T) {
	var st StackTrace
	func() {
		func() {
			st = testStackTraceInside()
		}()
	}()

	assert.Len(t, st, 3)
	assert.Equal(t, "location_stacktrace_test.go:24", st[0].String())
	assert.Equal(t, "location_stacktrace_test.go:25", st[1].String())
	assert.Equal(t, "location_stacktrace_test.go:33", st[2].String())

	re := `tlog.testStackTraceInside.func1                               at [\w.-/]*location_stacktrace_test.go:24
tlog.testStackTraceInside                                     at [\w.-/]*location_stacktrace_test.go:25
tlog.TestLocationStackTraceString.func1.1                     at [\w.-/]*location_stacktrace_test.go:33
`
	ok, err := regexp.MatchString(re, st.String())
	assert.NoError(t, err)
	assert.True(t, ok, "expected:\n%v\ngot:\n%v\n", re, st.String())
}

func TestLocationStackTraceFormat(t *testing.T) {
	var st StackTrace
	func() {
		func() {
			st = testStackTraceInside()
		}()
	}()

	assert.Equal(t, "location_stacktrace_test.go:24 at location_stacktrace_test.go:25 at location_stacktrace_test.go:55", fmt.Sprintf("%v", st))

	assert.Equal(t, "testStackTraceInside.func1:24 at testStackTraceInside:25 at TestLocationStackTraceFormat.func1.1:55", fmt.Sprintf("%#v", st))

	re := `at [\w.-/]*location_stacktrace_test.go:24
at [\w.-/]*location_stacktrace_test.go:25
at [\w.-/]*location_stacktrace_test.go:55
`
	v := fmt.Sprintf("%+v", st)
	assert.True(t, regexp.MustCompile(re).MatchString(v), "expected:\n%vgot:\n%v", re, v)
}
