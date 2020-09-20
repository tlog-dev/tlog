package tlogdb

import (
	"bytes"
	"encoding/binary"
	"encoding/json"

	"github.com/nikandfor/errors"
	"github.com/nikandfor/xrain"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/parse"
)

type (
	Writer struct {
		d *DB

		ls  tlog.Labels
		sls map[tlog.ID]tlog.Labels
	}
)

var _ parse.Writer = &Writer{}

func NewWriter(d *DB) (w *Writer, err error) {
	err = d.d.Update(func(tx *xrain.Tx) (err error) {
		for _, n := range []string{"m", "s", "f", "Lm", "Ls", "sL", "q", "sm", "i", "p", "c"} {
			_, err = tx.PutBucket([]byte(n))
			if err != nil {
				return
			}
		}

		return nil
	})
	if err != nil {
		return
	}

	w = &Writer{
		d:   d,
		sls: make(map[tlog.ID]tlog.Labels),
	}

	return w, nil
}

func (w *Writer) Labels(ls parse.Labels) (err error) {
	if ls.Span == (ID{}) {
		w.ls = ls.Labels

		return
	}

	data, err := json.Marshal(ls.Labels)
	if err != nil {
		return
	}

	w.sls[ls.Span] = ls.Labels

	err = w.d.d.Update(func(tx *xrain.Tx) (err error) {
		b := tx.Bucket([]byte("sL"))

		err = b.Put(ls.Span[:], data)
		if err != nil {
			return
		}

		id := tx.Bucket([]byte("i"))
		ts := id.Get(ls.Span[:])

		err = w.insertLabels(tx.Bucket([]byte("Ls")), ls.Span, ts, nil)
		if err != nil {
			return
		}

		return nil
	})

	return
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

		oldv := b.Get(tsbuf[:])
		if oldv != nil && bytes.Equal(oldv, data) {
			return nil
		}
		if oldv != nil {
			return errors.New("duplicated timestamp: %+v", m)
		}

		err = b.Put(tsbuf[:], data)
		if err != nil {
			return
		}

		if m.Span != (tlog.ID{}) {
			b = tx.Bucket([]byte("sm"))

			b, err = b.PutBucket(m.Span[:])
			if err != nil {
				return nil
			}

			err = b.Put(tsbuf[:], nil)
			if err != nil {
				return
			}
		}

		err = w.insertLabels(tx.Bucket([]byte("Lm")), m.Span, tsbuf[:], nil)
		if err != nil {
			return
		}

		err = w.insertQuery(tx, []byte(m.Text), tsbuf[:], nil)
		if err != nil {
			return
		}

		return nil
	})

	return
}

func (w *Writer) insertLabels(b *xrain.SimpleBucket, sid tlog.ID, k, v []byte) (err error) {
	var lb *xrain.SimpleBucket

	if sid == (tlog.ID{}) {
		for _, l := range w.ls {
			lb, err = b.PutBucket([]byte(l))
			if err != nil {
				return
			}

			err = lb.Put(k, v)
			if err != nil {
				return
			}
		}
		return
	}

	sls := w.sls[sid]

	for _, l := range sls {
		lb, err = b.PutBucket([]byte(l))
		if err != nil {
			return
		}

		err = lb.Put(k, v)
		if err != nil {
			return
		}
	}

	return nil
}

func (w *Writer) insertQuery(tx *xrain.Tx, q, key, val []byte) (err error) {
	if len(q) == 0 {
		return
	}

	qb := tx.Bucket([]byte("q"))

	for i := 0; i < len(q); i++ {
		b := qb

		for j := 0; i+j+qStep <= len(q) && i+j+qStep <= qMaxPrefix; j += qStep {
			pref := q[i+j : i+j+qStep]

			b, err = b.PutBucket(pref)
			if err != nil {
				return
			}

			err = b.Put(key, val) // TODO: avoid duplicates
			if err != nil {
				return
			}
		}
	}

	return nil
}

func (w *Writer) Metric(m parse.Metric) (err error) {
	return
}

func (w *Writer) Meta(m parse.Meta) (err error) {
	return
}

func (w *Writer) SpanStart(s parse.SpanStart) (err error) {
	var tsbuf [8]byte
	binary.BigEndian.PutUint64(tsbuf[:], uint64(s.Started))

	data, err := json.Marshal(s)
	if err != nil {
		return
	}

	err = w.d.d.Update(func(tx *xrain.Tx) (err error) {
		b := tx.Bucket([]byte("s"))

		if v := b.Get(tsbuf[:]); bytes.Equal(v, data) {
			return nil
		}

		err = b.Put(tsbuf[:], data)
		if err != nil {
			return
		}

		if s.Parent != (tlog.ID{}) {
			b = tx.Bucket([]byte("p"))

			err = b.Put(s.ID[:], s.Parent[:])
			if err != nil {
				return nil
			}

			b = tx.Bucket([]byte("c"))

			b, err = b.PutBucket(s.Parent[:])
			if err != nil {
				return nil
			}

			err = b.Put(tsbuf[:], s.ID[:])
			if err != nil {
				return nil
			}
		}

		err = w.insertLabels(tx.Bucket([]byte("Ls")), tlog.ID{}, tsbuf[:], nil)
		if err != nil {
			return
		}

		return nil
	})

	return
}

func (w *Writer) SpanFinish(f parse.SpanFinish) error {
	return nil
}
