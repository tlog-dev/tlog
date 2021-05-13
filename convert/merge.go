package convert

import (
	"bytes"

	"github.com/nikandfor/tlog/wire"
)

func Set(buf, msg []byte, data ...[]byte) []byte {
	var d wire.Decoder

	tag, els, i := d.Tag(msg, 0)

	if tag != wire.Map {
		return nil
	}

	buf = append(buf, wire.Map|wire.LenBreak)

out:
	for el := 0; els == -1 || el < int(els); el++ {
		if els == -1 && d.Break(msg, &i) {
			break
		}

		st := i

		i = d.Skip(msg, i)

		k := msg[st:i]

		i = d.Skip(msg, i) // val

		for _, d := range data {
			if bytes.HasPrefix(d, k) {
				continue out
			}
		}

		buf = append(buf, msg[st:i]...)
	}

	for _, d := range data {
		buf = append(buf, d...)
	}

	buf = append(buf, wire.Special|wire.Break)

	return buf
}
