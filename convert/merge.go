package convert

import (
	"bytes"

	"github.com/nikandfor/tlog"
)

func Set(buf, msg []byte, data ...[]byte) []byte {
	d := tlog.NewDecoderBytes(msg)

	var i int64
	tag, els, i := d.Tag(i)

	if tag != tlog.Map {
		return nil
	}

	buf = append(buf, tlog.Map|tlog.LenBreak)

out:
	for el := 0; els == -1 || el < els; el++ {
		if els == -1 && d.Break(&i) {
			break
		}

		st := i

		i = d.Skip(i)

		k := msg[st:i]

		i = d.Skip(i) // val

		for _, d := range data {
			if bytes.HasPrefix(d, k) {
				continue out
			}
		}

		buf = append(buf, msg[st:i]...)
	}

	if d.Err() != nil {
		return nil
	}

	for _, d := range data {
		buf = append(buf, d...)
	}

	buf = append(buf, tlog.Special|tlog.Break)

	return buf
}
