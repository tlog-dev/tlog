package convert

import (
	"io"

	"tlog.app/go/tlog/tlwire"
)

func Copy(w io.Writer, r io.Reader) (int64, error) {
	d := tlwire.NewReader(r)

	return d.WriteTo(w)
}
