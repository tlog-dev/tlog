package convert

import (
	"io"

	"github.com/nikandfor/errors"

	"github.com/nikandfor/tlog"
)

func Copy(w io.Writer, r io.Reader) (err error) {
	d := tlog.NewDecoder(r)

	i := int64(0)
	for {
		st := i
		d.Keep(st)

		i = d.Skip(st)
		if err = d.Err(); err == io.EOF {
			break
		} else if err != nil {
			return errors.Wrap(err, "read")
		}

		_, err = w.Write(d.Bytes(st, i))
		if err != nil {
			return errors.Wrap(err, "write")
		}
	}

	return nil
}
