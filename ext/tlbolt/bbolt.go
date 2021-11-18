package tlbolt

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"time"

	"github.com/nikandfor/errors"
	"github.com/nikandfor/loc"
	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/convert"
	"github.com/nikandfor/tlog/wire"
	"go.etcd.io/bbolt"
)

/*
	key ::= ts | labels_id

	"events" -> key => data

	"kv" -> field_name -> field_value -> key => ""

	"labels" -> label -> key => ""

	"L" -> labels_id => labels
*/

/*
    Selecting events

	SELECT * FROM events
	SELECT err FROM events GROUP BY err
	SELECT * FROM events WHERE m = "message"

	Selecting spans

	SELECT s FROM events WHERE m = "span_name"
	SELECT * FROM events WHERE s IN (SELECT s FROM events WHERE m = "span_name")

	List labels

	SELECT * FROM labels
*/

type (
	Writer struct {
		db *bbolt.DB

		d wire.Decoder

		ls tlog.Labels
	}

	header struct {
		ls tlog.Labels
		ts time.Time

		lssum  []byte
		labels []byte

		b [12]byte
	}

	bucket interface {
		Cursor() *bbolt.Cursor
		Bucket(k []byte) *bbolt.Bucket
	}
)

var tl *tlog.Logger

func NewWriter(db *bbolt.DB) (w *Writer) {
	w = &Writer{
		db: db,
	}

	return w
}

func (w *Writer) Write(p []byte) (i int, err error) {
	defer func() {
		tl.Printw("written", "err", err, "data", p, "callers", loc.Callers(1, 5))
	}()

	tag, els, i := w.d.Tag(p, i)
	if tag != wire.Map {
		return 0, errors.New("expected map")
	}

	h := w.findLabels(p, els, i)
	key := w.key(&h)

	err = w.db.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("events"))
		if err != nil {
			return errors.Wrap(err, "create bucket")
		}

		v := b.Get(key)
		if bytes.Equal(v, p) {
			return nil
		}
		if v != nil {
			return errors.New("duplicated key")
		}

		err = b.Put(key, p)
		if err != nil {
			return errors.Wrap(err, "put data")
		}

		if h.labels != nil {
			b, err = tx.CreateBucketIfNotExists([]byte("L"))
			if err != nil {
				return errors.Wrap(err, "create bucket")
			}

			err = b.Put(h.lssum, h.labels)
			if err != nil {
				return errors.Wrap(err, "put labels")
			}
		}

		b, err = tx.CreateBucketIfNotExists([]byte("labels"))
		if err != nil {
			return errors.Wrap(err, "create bucket")
		}

		for _, l := range h.ls {
			L, err := b.CreateBucketIfNotExists([]byte(l))
			if err != nil {
				return errors.Wrap(err, "create bucket")
			}

			err = L.Put(key, nil)
			if err != nil {
				return errors.Wrap(err, "put data")
			}
		}

		b, err = tx.CreateBucketIfNotExists([]byte("kv"))
		if err != nil {
			return errors.Wrap(err, "create bucket")
		}

		var k, ktp []byte
		for el := 0; els == -1 || el < int(els); el++ {
			if els == -1 && w.d.Break(p, &i) {
				break
			}

			k, i = w.d.String(p, i)

			st := i

			i = w.d.Skip(p, st)

			v = p[st:i]

			tag, sub, tpi := w.d.Tag(p, st)
			if tag == wire.Semantic {
				if sub == tlog.WireLabels && string(k) == tlog.KeyLabels {
					continue
				}
				if sub == wire.Time && string(k) == tlog.KeyTime {
					continue
				}
			}

			ktp = append(ktp[:0], k...)

			switch tag {
			case wire.Int, wire.Neg:
				ktp = append(ktp, wire.Int)
			case wire.String, wire.Bytes, wire.Array, wire.Map:
				ktp = append(ktp, byte(tag))
			case wire.Semantic:
				ktp = append(ktp, p[st:tpi]...)
			case wire.Special:
				switch sub {
				case wire.False, wire.True:
					ktp = append(ktp, wire.Special|wire.False)
				case wire.Nil, wire.Undefined:
					ktp = append(ktp, p[st:tpi]...)
				case wire.Float8, wire.Float16, wire.Float32, wire.Float64:
					ktp = append(ktp, wire.Special|wire.Float64)
				default:
					panic("special")
				}
			default:
				panic("type")
			}

			keyb, err := b.CreateBucketIfNotExists(ktp)
			if err != nil {
				return errors.Wrap(err, "create bucket")
			}

			valb, err := keyb.CreateBucketIfNotExists(v)
			if err != nil {
				return errors.Wrap(err, "create bucket")
			}

			err = valb.Put(key, nil)
			if err != nil {
				return errors.Wrap(err, "put data")
			}
		}

		return nil
	})
	if err != nil {
		return 0, errors.Wrap(err, "db")
	}

	w.ls = h.ls

	return len(p), nil
}

func (w *Writer) Events(q string, n int, token, buf []byte) (res [][]byte, next []byte, err error) {
	reverse := n <= 0
	if n < 0 {
		n = -n
	}

	buf = buf[:0]

	err = w.db.View(func(tx *bbolt.Tx) (err error) {
		L := tx.Bucket([]byte("L"))

		b := tx.Bucket([]byte("events"))
		if b == nil {
			return nil
		}

		c := b.Cursor()

		var k, v []byte
		if token == nil {
			if reverse {
				k, v = c.Last()
			} else {
				k, v = c.First()
			}
		} else {
			k, v = c.Seek(token)

			if reverse {
				k, v = c.Prev()
			}
		}

		st := 0
		for k != nil {
			st = len(buf)

			var labels []byte
			if len(k) >= 12 && L != nil {
				labels = L.Get(k[8:12])
			}

			if labels != nil {
				buf = convert.Set(buf, v, labels)
			} else {
				buf = append(buf, v...)
			}

			res = append(res, buf[st:])

			if !reverse {
				k, v = c.Next()
			}

			if len(res) == n {
				if k != nil {
					st = len(buf)
					buf = append(buf, k...)

					next = buf[st:]
				}

				break
			}

			if reverse {
				k, v = c.Prev()
			}
		}

		return nil
	})

	return
}

func (w *Writer) Dump(wr io.Writer) error {
	return w.db.View(func(tx *bbolt.Tx) (err error) {
		return w.dump(wr, tx, 0)
	})
}

func (w *Writer) dump(wr io.Writer, b bucket, d int) (err error) {
	c := b.Cursor()

	for k, v := c.First(); k != nil; k, v = c.Next() {
		fmt.Fprintf(wr, "%s", "                                                                    "[:d*4])

		if v != nil {
			fmt.Fprintf(wr, "%q => %q\n", k, v)
			continue
		}

		fmt.Fprintf(wr, "%q ->\n", k)

		err = w.dump(wr, b.Bucket(k), d+1)
		if err != nil {
			return err
		}
	}

	return nil
}

func (w *Writer) findLabels(p []byte, els int64, i int) (h header) {
	h.ls = w.ls

	var k []byte
	for el := 0; els == -1 || el < int(els); el++ {
		if els == -1 && w.d.Break(p, &i) {
			break
		}

		kst := i

		k, i = w.d.String(p, i)

		tag, sub, _ := w.d.Tag(p, i)
		if tag != wire.Semantic {
			i = w.d.Skip(p, i)
			continue
		}

		switch {
		case sub == tlog.WireLabels && string(k) == tlog.KeyLabels:
			i = h.ls.TlogParse(&w.d, p, i)

			h.labels = p[kst:i]
		case sub == wire.Time && string(k) == tlog.KeyTime:
			h.ts, i = w.d.Time(p, i)
		default:
			i = w.d.Skip(p, i)
		}
	}

	return
}

func (w *Writer) key(h *header) []byte {
	binary.BigEndian.PutUint64(h.b[:], uint64(h.ts.UnixNano()))

	var sum uint32
	for _, l := range h.ls {
		sum = crc32.Update(sum, crc32.IEEETable, []byte(l))
	}

	binary.BigEndian.PutUint32(h.b[8:], sum)

	h.lssum = h.b[8:]

	return h.b[:]
}
