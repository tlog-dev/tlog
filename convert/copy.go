package convert

import (
	"io"

	"github.com/nikandfor/tlog/tlwire"
)

func Copy(w io.Writer, r io.Reader) (int64, error) {
	d := tlwire.NewStreamDecoder(r)

	return d.WriteTo(w)
}
