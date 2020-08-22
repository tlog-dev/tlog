// +build ignore

package tlogdb

import (
	"bytes"
	"encoding/binary"
	"encoding/json"

	"github.com/nikandfor/xrain"

	"github.com/nikandfor/tlog/parse"
)

type (
	Writer struct {
		d *DB
	}
)

func NewWriter(d *DB) (w *Writer, err error) {
	err = d.d.Update(func(tx *xrain.Tx) (err error) {
		_, err = tx.PutBucket([]byte("m"))
		if err != nil {
			return
		}

		_, err = tx.PutBucket([]byte("q"))
		if err != nil {
			return
		}

		return nil
	})
	if err != nil {
		return
	}

	w = &Writer{
		d: d,
	}

	return w, nil
}

func (w *Writer) Labels(ls parse.Labels) error {
	return nil
}

func (w *Writer) Location(l parse.Location) error {
	return nil
}

func (w *Writer) Message(m parse.Message) (err error) {
	var tsbuf [8]byte
	binary.BigEndian.PutUint64(tsbuf[:], uint64(m.Time))

	data, err := json.Marshal(m)
	if err != nil {
		return
	}

	err = w.d.d.Update(func(tx *xrain.Tx) (err error) {
		b := tx.Bucket([]byte("m"))

		if v := b.Get(tsbuf[:]); bytes.Equal(v, data) {
			return nil
		}

		err = b.Put(tsbuf[:], data)
		if err != nil {
			return
		}

		if m.Text != "" {
			qq := []byte(m.Text)
			qb := tx.Bucket([]byte("q"))

			for i := 0; i < len(qq)-1; i++ {
				pref := qq[i:]
				if len(pref) > qstep {
					pref = pref[:qstep]
				}

				qpb, err := qb.PutBucket(pref)
				if err != nil {
					return err
				}

				err = qpb.Put(tsbuf[:], nil) // TODO: avoid duplicates
				if err != nil {
					return err
				}
			}
		}

		return nil
	})

	return
}

func (w *Writer) Metric(m parse.Metric) (err error) {
	return
}

func (w *Writer) SpanStart(s parse.SpanStart) error {
	return nil
}

func (w *Writer) SpanFinish(f parse.SpanFinish) error {
	return nil
}
