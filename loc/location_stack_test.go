package loc

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLocationFillCallers(t *testing.T) {
	st := make(PCs, 1)

	st = CallersFill(0, st)

	assert.Len(t, st, 1)
	assert.Equal(t, "location_stack_test.go:14", st[0].String())
}

func testLocationsInside() (st PCs) {
	func() {
		func() {
			st = Callers(1, 3)
		}()
	}()
	return
}

func TestLocationPCsString(t *testing.T) {
	var st PCs
	func() {
		func() {
			st = testLocationsInside()
		}()
	}()

	assert.Len(t, st, 3)
	assert.Equal(t, "location_stack_test.go:24", st[0].String())
	assert.Equal(t, "location_stack_test.go:25", st[1].String())
	assert.Equal(t, "location_stack_test.go:33", st[2].String())

	re := `loc.testLocationsInside.func1                                 at [\w.-/]*location_stack_test.go:24
loc.testLocationsInside                                       at [\w.-/]*location_stack_test.go:25
loc.TestLocationPCsString.func1.1                             at [\w.-/]*location_stack_test.go:33
`
	ok, err := regexp.MatchString(re, st.String())
	assert.NoError(t, err)
	assert.True(t, ok, "expected:\n%v\ngot:\n%v\n", re, st.String())
}

func TestLocationPCsFormat(t *testing.T) {
	var st PCs
	func() {
		func() {
			st = testLocationsInside()
		}()
	}()

	assert.Equal(t, "location_stack_test.go:24 at location_stack_test.go:25 at location_stack_test.go:55", fmt.Sprintf("%v", st))

	assert.Equal(t, "testLocationsInside.func1:24 at testLocationsInside:25 at TestLocationPCsFormat.func1.1:55", fmt.Sprintf("%#v", st))

	re := `at [\w.-/]*location_stack_test.go:24
at [\w.-/]*location_stack_test.go:25
at [\w.-/]*location_stack_test.go:55
`
	v := fmt.Sprintf("%+v", st)
	assert.True(t, regexp.MustCompile(re).MatchString(v), "expected:\n%vgot:\n%v", re, v)
}
