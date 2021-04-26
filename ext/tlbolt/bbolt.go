package tlbolt

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"

	"github.com/nikandfor/errors"
	"github.com/nikandfor/loc"
	"github.com/nikandfor/tlog"
	"go.etcd.io/bbolt"
)

/*
	key ::= ts labels_id

	"e" -> key => data

	"v" -> field_name -> field_value -> key => ""

	"L" -> label -> key => ""
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

		d tlog.Decoder

		ls tlog.Labels
	}

	header struct {
		ls tlog.Labels
		ts tlog.Timestamp
	}

	bucket interface {
		Cursor() *bbolt.Cursor
		Bucket(k []byte) *bbolt.Bucket
	}
)

var tl *tlog.Logger

func New(db *bbolt.DB) (w *Writer) {
	w = &Writer{
		db: db,
	}

	return w
}

func (w *Writer) Write(p []byte) (_ int, err error) {
	defer func() {
		tl.Printw("written", "err", err, "data", p, "callers", loc.Callers(1, 5))
	}()

	var i int64
	w.d.ResetBytes(p)

	tag, els, i := w.d.Tag(i)
	if err = w.d.Err(); err != nil {
		return
	}

	if tag != tlog.Map {
		return 0, errors.New("expected map")
	}

	h := w.findLabels(els, i)
	key := w.key(h)

	err = w.db.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("e"))
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

		b, err = tx.CreateBucketIfNotExists([]byte("L"))
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

		b, err = tx.CreateBucketIfNotExists([]byte("v"))
		if err != nil {
			return errors.Wrap(err, "create bucket")
		}

		var k, ktp []byte
		for el := 0; els == -1 || el < els; el++ {
			if els == -1 && w.d.Break(&i) {
				break
			}

			k, i = w.d.String(i)

			st := i

			i = w.d.Skip(st)
			if err = w.d.Err(); err != nil {
				return errors.Wrap(err, "decode key-value")
			}

			v = p[st:i]

			tag, sub, tpi := w.d.Tag(st)
			if tag == tlog.Semantic {
				if sub == tlog.WireLabels && string(k) == tlog.KeyLabels {
					continue
				}
				if sub == tlog.WireTime && string(k) == tlog.KeyTime {
					continue
				}
			}

			ktp = append(ktp[:0], k...)

			switch tag {
			case tlog.Int, tlog.Neg:
				ktp = append(ktp, tlog.Int)
			case tlog.String, tlog.Bytes, tlog.Array, tlog.Map:
				ktp = append(ktp, byte(tag))
			case tlog.Semantic:
				ktp = append(ktp, p[st:tpi]...)
			case tlog.Special:
				switch sub {
				case tlog.False, tlog.True:
					ktp = append(ktp, tlog.Special|tlog.False)
				case tlog.Null, tlog.Undefined:
					ktp = append(ktp, p[st:tpi]...)
				case tlog.FloatInt8, tlog.Float16, tlog.Float32, tlog.Float64:
					ktp = append(ktp, tlog.Special|tlog.Float64)
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
	reverse := n >= 0
	if n < 0 {
		n = -n
	}

	buf = buf[:0]

	err = w.db.View(func(tx *bbolt.Tx) (err error) {
		b := tx.Bucket([]byte("e"))
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
			buf = append(buf, v...)

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

func (w *Writer) findLabels(els int, i int64) (h header) {
	h.ls = w.ls

	var k []byte
	for el := 0; els == -1 || el < els; el++ {
		if els == -1 && w.d.Break(&i) {
			break
		}

		k, i = w.d.String(i)

		tag, sub, _ := w.d.Tag(i)
		if tag != tlog.Semantic {
			i = w.d.Skip(i)
			continue
		}

		switch {
		case sub == tlog.WireLabels && string(k) == tlog.KeyLabels:
			h.ls, i = w.d.Labels(i)
		case sub == tlog.WireTime && string(k) == tlog.KeyTime:
			h.ts, i = w.d.Time(i)
		default:
			i = w.d.Skip(i)
		}
	}

	return
}

func (w *Writer) key(h header) []byte {
	var b [12]byte

	binary.BigEndian.PutUint64(b[:], uint64(h.ts))

	var sum uint32
	for _, l := range w.ls {
		sum = crc32.Update(sum, crc32.IEEETable, []byte(l))
	}

	binary.BigEndian.PutUint32(b[8:], sum)

	return b[:]
}
