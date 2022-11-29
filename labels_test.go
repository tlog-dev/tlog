package tlog

import (
	"testing"

	"github.com/nikandfor/assert"
)

func TestParseLabels(t *testing.T) {
	l := ParseLabels("_hostname,_user,a=b,c=4")
	assert.Equal(t, []interface{}{"_hostname", Hostname(), "_user", User(), "a", "b", "c", 4}, l)
}
