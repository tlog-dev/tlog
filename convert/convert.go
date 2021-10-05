package convert

import (
	"fmt"
	"io"

	"github.com/nikandfor/errors"
	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/wire"
)

func Copy(w io.Writer, r io.Reader) (err error) {
	d := wire.NewStreamDecoder(r)

	//	println("coooooopy")

	for {
		data, err := d.Decode()
		if err != nil && errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if tlog.If("decode_dump") {
				fmt.Fprintf(tlog.Stderr, "err %v\n%v", err, wire.Dump(data))
			}
			return errors.Wrap(err, "read")
		}

		_, err = w.Write(data)
		if err != nil {
			return errors.Wrap(err, "write")
		}
	}

	return nil
}
