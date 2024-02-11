//go:build ignore

package tlclick

import (
	"context"
	"encoding/hex"
	"io"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"
	"tlog.app/go/errors"
	"tlog.app/go/loc"
	"tlog.app/go/tlog"
	"tlog.app/go/tlog/convert"
	"tlog.app/go/tlog/tlwire"
)

type (
	UUID = uuid.UUID

	Click struct {
		c driver.Conn

		KeyTimestamp string
		KeySpan      string

		JSON *convert.JSON

		b         driver.Batch
		lastFlush time.Time

		spans   []UUID
		related []UUID

		dataJSON   []byte
		labelsJSON []byte

		labelsBuf []byte
		labelsArr [][]byte
	}
)

func New(c driver.Conn) *Click {
	d := &Click{
		c: c,

		KeyTimestamp: tlog.KeyTimestamp,
		KeySpan:      tlog.KeySpan,
	}

	d.JSON = convert.NewJSON(nil)
	d.JSON.AppendNewLine = false

	return d
}

func (d *Click) Query(ctx context.Context, w io.Writer, ts int64, q string) error {
	return nil
}

func (d *Click) CreateTables(ctx context.Context) error {
	err := d.c.Exec(ctx, `CREATE TABLE IF NOT EXISTS events (
	tlog String,
	json String,

	labels       String COMMENT 'json formatted',
	_labels      Array(String) COMMENT 'tlog pairs',
	_labels_hash UInt64 MATERIALIZED cityHash64(arrayStringConcat(_labels)),

	ts DateTime64(9, 'UTC'),

	spans   Array(UUID),
	related Array(UUID),

	timestamp String ALIAS visitParamExtractString(json, '_t'),
	span      String ALIAS toUUIDOrZero(visitParamExtractString(json, '_s')),
	parent    String ALIAS toUUIDOrZero(visitParamExtractString(json, '_p')),
	caller    String ALIAS visitParamExtractString(json, '_c'),
	message   String ALIAS visitParamExtractString(json, '_m'),
	msg       String ALIAS message,
	event     String ALIAS visitParamExtractString(json, '_k'),
	elapsed   Int64  ALIAS visitParamExtractInt(json, '_e'),
	log_level Int8   ALIAS visitParamExtractInt(json, '_l'),
	error     String ALIAS visitParamExtractString(json, 'err'),

	kvs Array(Tuple(String, String)) ALIAS arrayMap(k -> (k, JSONExtractRaw(json, k)), arrayFilter(k -> k NOT IN ('_s', '_t', '_c', '_m'), JSONExtractKeys(json))),

	minute  DateTime ALIAS        toStartOfMinute(ts),
	hour    DateTime ALIAS        toStartOfHour(ts),
	day     Date     MATERIALIZED toStartOfDay(ts),
	week    Date     ALIAS        toStartOfWeek(ts),
	month   Date     ALIAS        toStartOfMonth(ts),
	year    Date     ALIAS        toStartOfYear(ts),
)
ENGINE ReplacingMergeTree
ORDER BY ts
PARTITION BY (day, _labels_hash)
`)
	if err != nil {
		return errors.Wrap(err, "events")
	}

	err = d.c.Exec(ctx, `CREATE OR REPLACE VIEW spans AS
SELECT
	span,
	min(ts) AS start,
	max(ts) AS end,
	anyIf(message, event = 's') AS name,
	round(anyIf(elapsed, event = 'f') / 1e9, 1) AS elapsed_s,
	anyIf(error, event = 'f') AS error
FROM events GROUP BY span
`)
	if err != nil {
		return errors.Wrap(err, "spans")
	}

	return nil
}

func (d *Click) Write(p []byte) (n int, err error) {
	for n < len(p) {
		n, err = d.writeEvent(p, n)
		if err != nil {
			return n, err
		}
	}

	return
}

func (d *Click) writeEvent(p []byte, st int) (next int, err error) {
	if tlog.If("dump") {
		defer func() {
			tlog.Printw("event", "st", tlog.NextAsHex, st, "next", tlog.NextAsHex, next, "err", err, "msg", tlog.RawMessage(p))
		}()
	}

	ts, next, err := d.parseEvent(p, st)
	if err != nil {
		return st, errors.Wrap(err, "parse message")
	}

	if d.b == nil {
		ctx := context.Background()

		d.b, err = d.c.PrepareBatch(ctx, `INSERT INTO events (
			tlog, json,
			_labels, labels,
			ts,
			spans, related
		) VALUES`)
		if err != nil {
			return st, errors.Wrap(err, "prepare batch")
		}

		d.lastFlush = time.Now()
	}

	err = d.b.Append(
		p[st:next], d.dataJSON,
		d.labelsArr, d.labelsJSON,
		time.Unix(0, ts).UTC(),
		d.spans, d.related,
	)
	if err != nil {
		return st, errors.Wrap(err, "append row")
	}

	if time.Since(d.lastFlush) > 1000*time.Millisecond {
		err = d.Flush()
		if err != nil {
			return st, errors.Wrap(err, "flush")
		}
	}

	return next, nil
}

func (d *Click) Flush() (err error) {
	if d.b != nil {
		e := d.b.Send()
		tlog.Printw("flush", "err", e)
		if err == nil {
			err = errors.Wrap(e, "flush batch")
		}

		d.b = nil
	}

	return err
}

func (d *Click) Close() error {
	return d.Flush()
}

func (d *Click) parseEvent(p []byte, st int) (ts int64, i int, err error) {
	defer func() {
		pe := recover()
		if pe == nil {
			return
		}

		tlog.V("panic_dump_hex").Printw("panic", "panic", pe, "st", tlog.NextAsHex, st, "pos", tlog.NextAsHex, i, "buf", hex.Dump(p))
		tlog.V("panic_dump_wire").Printw("panic", "panic", pe, "st", tlog.NextAsHex, st, "pos", tlog.NextAsHex, i, "dump", tlwire.Dump(p))
		tlog.Printw("panic", "panic", pe, "from", loc.Callers(1, 10))

		panic(pe)
	}()

	var dec tlwire.Decoder

	d.spans = d.spans[:0]
	d.related = d.related[:0]

	d.dataJSON = d.dataJSON[:0]
	d.labelsJSON = d.labelsJSON[:0]

	d.labelsBuf = d.labelsBuf[:0]
	d.labelsArr = d.labelsArr[:0]

	defer func() {
		if len(d.dataJSON) != 0 {
			d.dataJSON = append(d.dataJSON, '}')
		}

		if len(d.labelsJSON) != 0 {
			d.labelsJSON = append(d.labelsJSON, '}')
		}
	}()

	tag, els, i := dec.Tag(p, st)
	if tag != tlwire.Map {
		err = errors.New("expected map")
		return
	}

	var k []byte
	var sub int64
	var end int

	for el := 0; els == -1 || el < int(els); el++ {
		if els == -1 && dec.Break(p, &i) {
			break
		}

		st := i

		k, i = dec.Bytes(p, i)
		if len(k) == 0 {
			err = errors.New("empty key")
			return
		}

		tag, sub, end = dec.SkipTag(p, i)

		{
			var jb []byte

			if tag == tlwire.Semantic && sub == tlog.WireLabel {
				jb = d.labelsJSON
			} else {
				jb = d.dataJSON
			}

			if len(jb) == 0 {
				jb = append(jb, '{')
			} else {
				jb = append(jb, ',')
			}

			jb, _ = d.JSON.ConvertValue(jb, p, st)

			jb = append(jb, ':')

			jb, _ = d.JSON.ConvertValue(jb, p, i)

			if tag == tlwire.Semantic && sub == tlog.WireLabel {
				d.labelsJSON = jb
			} else {
				d.dataJSON = jb
			}
		}

		if tag != tlwire.Semantic {
			i = end
			continue
		}

		switch {
		case sub == tlwire.Time && string(k) == d.KeyTimestamp:
			ts, i = dec.Timestamp(p, i)
		case sub == tlog.WireLabel:
			// labels = crc32.Update(labels, crc32.IEEETable, p[st:end])

			lst := len(d.labelsBuf)
			d.labelsBuf = append(d.labelsBuf, p[st:end]...)
			d.labelsArr = append(d.labelsArr, d.labelsBuf[lst:])
		case sub == tlog.WireID:
			var id tlog.ID
			_ = id.TlogParse(p, i)

			u := UUID(id)

			//tlog.Printw("parsed id", "id", id, "key", string(k), "key_span", d.KeySpan)

			if string(k) == d.KeySpan {
				d.spans = append(d.spans, u)
			} else {
				d.related = append(d.related, u)
			}
		}

		i = end
	}

	return
}
