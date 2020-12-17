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
		if i != 0 {
			i = copy(b, b[i:])
		}

		n, err := r.Read(b[i:])
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return errors.Wrap(err, "read")
		}

		//	tlog.Printf("read from %x another %x byte", i, n)

		n += i
		i = 0

		d.ResetBytes(b[:n])

		for i < n { // loop over the buffer
			end := d.Skip(i)

			//	tlog.Printf("skip %x to end %x  of %x  err %v", i, end, n, d.Err())

			e := d.Err()
			if errors.Is(e, io.ErrUnexpectedEOF) {
				continue file
			}

			if err != nil {
				return errors.Wrap(err, "parse")
			}

			_, e = w.Write(b[i:end])
			if e != nil {
				return errors.Wrap(e, "write")
			}

			i = end
		}
	}
}
