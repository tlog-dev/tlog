package tlbolt

import (
	"errors"

	"github.com/nikandfor/tlog"
	"go.etcd.io/bbolt"
)

/*
	key ::= ts labels_id

	"e" -> key => data

	"i" -> field_name -> field_value -> key => ""

	"L" -> label -> key => ""
*/

type (
	Writer struct {
		db *bbolt.DB

		d tlog.Decoder

		ls, tmpls tlog.Labels
	}
)

func (w *Writer) Write(p []byte) (_ int, err error) {
	i := 0
	w.d.ResetBytes(p)

again:
	tag, els, i := w.d.Tag(i)
	if err = w.d.Err(); err != nil {
		return
	}

	if tag == Semantic && els == WireHeader {
		i = w.d.Skip(i)
		goto again
	}

	if tag != Map {
		return 0, errors.New("expected map")
	}

	err = w.db.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(""))
		if err != nil {
			return errors.Wrap(err, "create bucket")
		}

		var k, v []byte
		for el := 0; els == -1 || el < els; el++ {
			if els == -1 && w.d.Break(&i) {
				break
			}

			k, i = w.d.String(i)

			st := i
			i = w.d.Skip(st)

			if err = w.d.Err(); err != nil {
				return 0, errors.Wrap(err, "decode key-value")
			}

		}
	})

	if w.tmpls != nil {
		w.ls = w.tmpls
		w.tmpls = nil
	}

	return len(p), nil
}
