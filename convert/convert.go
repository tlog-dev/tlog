package convert

import (
	"io"

	"github.com/nikandfor/errors"
	"github.com/nikandfor/tlog/wire"
)

func Copy(w io.Writer, r io.Reader) (err error) {
	d := wire.NewStreamDecoder(r)

	//	println("coooooopy")

	for {
		d.Keep(true)

		//	println("looop")

		d.Skip()
		if err = d.Err(); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			//	fmt.Fprintf(tlog.Stderr, "error: %v\ndump\n%s\n", err, wire.Dump(d.Bytes()))
			return errors.Wrap(err, "read")
		}

		//	fmt.Fprintf(tlog.Stderr, "copy %x %x\n%s", d.Ref(), d.I(), wire.Dump(d.Bytes()))

		_, err = w.Write(d.Bytes())
		if err != nil {
			return errors.Wrap(err, "write")
		}
	}

	return nil
}
