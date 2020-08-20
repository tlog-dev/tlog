//nolint
package tlogdb

import (
	"encoding/binary"
	"encoding/json"

	"github.com/nikandfor/xrain"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/parse"
)

/*
	DB structure:

	ts ::= 8 byte timestamp
	-> means bucket
	=> means key-value pair

	m -> <ts> => <message>
	s -> <ts> => <span_start>
	f -> <ts> => <span_finish>

	i -> <span_id> => <span_ts>
	p -> <span_id> => <parent_span_id>
	c -> <span_id> -> <child_span_ts> => <child_span_id>

	M -> <span_id> -> <message_ts> =>

	q -> <pref> -> <pref> -> <pref> -> <message_ts> =>
*/

type (
	DB struct {
		d *xrain.DB
	}

	Writer struct {
		d *DB
	}

	Event struct {
		*parse.SpanStart
		*parse.SpanFinish
		*parse.Message
	}

	stream struct {
		c chan []byte

		top []byte

		stopc struct{}
	}
)

var _ parse.Writer = &Writer{}

var tl *tlog.Logger

func NewDB(db *xrain.DB) *DB {
	return &DB{d: db}
}

func (d *DB) All(it []byte, n int) (evs []Event, next []byte, err error) {
	var vbuf []byte

	err = d.d.View(func(tx *xrain.Tx) (err error) {
		b := tx.Bucket([]byte("m"))
		if b == nil {
			tl.Printf("no 'm' bucket")
			return
		}

		t := b.Tree()

		tl.Printf("tree %+v  it %x  n %v", t, it, n)

		for st, _ := t.Seek(it, nil, nil); st != nil; st = t.Next(st) {
			tl.Printf("st %v", st)
			if len(evs) == n {
				next, _ = t.Key(st, vbuf[:0])

				break
			}

			vbuf = t.Value(st, vbuf[:0])

			var m parse.Message

			err = json.Unmarshal(vbuf, &m)
			if err != nil {
				tl.Printf("unmarshal %v", err)
				return
			}

			evs = append(evs, Event{Message: &m})
		}

		tl.Printf("next %x", next)

		return nil
	})

	return
}

func (d *DB) messages(tx *xrain.Tx, seek []byte, stopc chan struct{}) (c chan []byte, errc chan error) {
	return
}

func (d *DB) merge(a, b chan []byte, stopc chan struct{}) (c chan []byte, errc chan error) {
	return
}

func (d *DB) substract(a, b chan []byte, stopc chan struct{}) (c chan []byte, errc chan error) {
	return
}

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

		err = b.Put(tsbuf[:], data)
		if err != nil {
			return
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
