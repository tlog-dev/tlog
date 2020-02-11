package parse

import (
	"io"
	"testing"

	"github.com/nikandfor/tlog"
)

func TestJSONReader(t *testing.T) {
	testReader(t,
		func(w io.Writer) tlog.Writer { return tlog.NewJSONWriter(w) },
		func(r io.Reader) Reader { return NewJSONReader(r) },
	)
}
