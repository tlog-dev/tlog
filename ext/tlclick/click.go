package tlclick

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"
	"github.com/nikandfor/errors"
	"github.com/nikandfor/loc"
	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/convert"
	"github.com/nikandfor/tlog/tlwire"
)

type (
	UUID = uuid.UUID

	Click struct {
		c driver.Conn

		Table string

		KeyTimestamp string
		KeySpan      string

		JSON *convert.JSON

		b driver.Batch

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
		c:     c,
		Table: "logs",

		KeyTimestamp: tlog.KeyTimestamp,
		KeySpan:      tlog.KeySpan,
	}

	d.JSON = convert.NewJSON(nil)
	d.JSON.AppendNewLine = false

	return d
}

func (d *Click) Query(ctx context.Context, w io.Writer, q string) error {
	return nil
}

func (d *Click) CreateTables(ctx context.Context) error {
	err := d.c.Exec(ctx, fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
	tlog String,
	json String,

	labels       String COMMENT 'json formatted',
	_labels      Array(String) COMMENT 'tlog pairs',
	_labels_hash UInt64 MATERIALIZED cityHash64(_labels),

	ts DateTime64(9, 'UTC'),

	spans   Array(UUID),
	related Array(UUID),

	span      String ALIAS toUUIDOrZero(visitParamExtractString(json, '_s')),
	parent    String ALIAS toUUIDOrZero(visitParamExtractString(json, '_p')),
	caller    String ALIAS visitParamExtractString(json, '_c'),
	message   String ALIAS visitParamExtractString(json, '_m'),
	event     String ALIAS visitParamExtractString(json, '_k'),
	elapsed   Int64  ALIAS visitParamExtractInt(json, '_e'),
	log_level Int8   ALIAS visitParamExtractInt(json, '_l'),

	minute  DateTime ALIAS        toStartOfMinute(ts),
	hour    DateTime ALIAS        toStartOfHour(ts),
	day     Date     MATERIALIZED toStartOfDay(ts),
	week    Date     ALIAS        toStartOfWeek(ts),
	month   Date     ALIAS        toStartOfMonth(ts),
	year    Date     ALIAS        toStartOfYear(ts),
)
ENGINE MergeTree
ORDER BY ts
PARTITION BY (day, _labels_hash)
`, d.Table))
	if err != nil {
		return errors.Wrap(err, d.Table)
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
	defer func() {
		tlog.Printw("write event", "err", err, "spans", d.spans, "from", loc.Caller(1))
	}()

	ts, next, err := d.parseEvent(p, st)
	tlog.V("dump").Printw("message", "ts", ts, "parse_err", err, "msg", tlog.RawMessage(p))
	if err != nil {
		return st, errors.Wrap(err, "parse message")
	}

	if d.b == nil {
		ctx := context.Background()

		d.b, err = d.c.PrepareBatch(ctx, fmt.Sprintf(`INSERT INTO %s (
			tlog, json,
			_labels, labels,
			ts,
			spans, related
		) VALUES`, d.Table))
		if err != nil {
			return st, errors.Wrap(err, "prepare batch")
		}
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

	err = d.b.Send()
	if err != nil {
		return st, errors.Wrap(err, "send")
	}

	d.b = nil

	return next, nil
}

func (d *Click) parseEvent(p []byte, st int) (ts int64, i int, err error) {
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
		if tag != tlwire.Semantic {
			i = dec.Skip(p, i)
			continue
		}

		{
			jb := d.dataJSON

			if sub == tlog.WireLabel {
				jb = d.labelsJSON
			}

			if len(jb) == 0 {
				jb = append(jb, '{')
			} else {
				jb = append(jb, ',')
			}

			jb, _ = d.JSON.ConvertValue(jb, p, st)

			jb = append(jb, ':')

			jb, _ = d.JSON.ConvertValue(jb, p, i)

			if sub == tlog.WireLabel {
				d.labelsJSON = jb
			} else {
				d.dataJSON = jb
			}
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

			tlog.Printw("parsed id", "id", id, "key", string(k), "key_span", d.KeySpan)

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
