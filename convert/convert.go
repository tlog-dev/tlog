package convert

import (
	"io"

	"github.com/nikandfor/errors"
	"github.com/nikandfor/tlog"
)

type (
	Convertor struct {
		d tlog.Decoder
		b []byte
	}
)

func Copy(w io.Writer, r io.Reader) (err error) {
	var d tlog.Decoder

	b := make([]byte, 16*1024)

	i := 0

file:
	for { // loop over the file
		n, err := r.Read(b[i:])
		if errors.Is(err, io.EOF) {
			if n == 0 {
				if i == 0 {
					return nil
				} else {
					return io.ErrUnexpectedEOF
				}
			}
		} else if err != nil {
			return errors.Wrap(err, "read")
		}

		//	tlog.Printf("read from %4x another %4x bytes [% .6x]", i, n, b[i:])

		n += i
		i = 0

		d.ResetBytes(b[:n])

		for i < n { // loop over the buffer
			end := d.Skip(i)

			//	tlog.Printf("skip from %4x to end %4x  of %4x  err %v", i, end, n, d.Err())

			err = d.Err()
			if errors.Is(err, io.ErrUnexpectedEOF) {
				i = copy(b, b[i:n])

				continue file
			}

			if err != nil {
				return errors.Wrap(err, "parse")
			}

			_, err = w.Write(b[i:end])
			if err != nil {
				return errors.Wrap(err, "write")
			}

			i = end
		}

		i = 0
	}
}
