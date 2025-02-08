package tlclick

import (
	"context"
	_ "embed"
	"io"
	"sync"
	"time"

	"github.com/ClickHouse/ch-go"
	"github.com/ClickHouse/ch-go/chpool"
	"github.com/ClickHouse/ch-go/proto"
	"tlog.app/go/errors"
	"tlog.app/go/tlog"

	"tlog.app/go/tlog/convert"
	"tlog.app/go/tlog/tlwire"
)

//go:embed schema2.sql
var schema []byte

type (
	Click struct {
		pool *chpool.Pool

		d tlwire.Decoder
		j convert.JSON

		mu sync.Mutex

		json   []byte
		labels []byte

		ls [][]byte // tlog labels

		pair []byte
		buf  []byte

		cols cols
	}

	cols struct {
		tlog proto.ColBytes
		ls   *proto.ColArr[[]byte]
		ts   proto.ColDateTime64Raw

		json   proto.ColBytes
		labels proto.ColBytes

		input proto.Input
		query ch.Query
	}
)

func New(pool *chpool.Pool) *Click {
	d := &Click{pool: pool}

	c := &d.cols

	c.input = proto.Input{
		{Name: "tlog", Data: &c.tlog},
		{Name: "_labels", Data: proto.NewArray(&proto.ColStr{})},
		{Name: "ts", Data: &c.ts},
		{Name: "json", Data: &c.json},
		{Name: "labels", Data: &c.labels},
	}

	c.query = ch.Query{
		Body:  c.input.Into("ingest"),
		Input: c.input,
	}

	return d
}

func (d *Click) Write(p []byte) (int, error) {
	defer d.mu.Unlock()
	d.mu.Lock()

	d.json = d.json[:0]
	d.labels = d.labels[:0]
	d.ls = d.ls[:0]
	d.buf = d.buf[:0]

	tag, els, i := d.d.Tag(p, 0)
	if tag != tlwire.Map {
		return 0, errors.New("expected map")
	}

	var k []byte
	var ts int64

	for el := 0; els == -1 || el < int(els); el++ {
		if els == -1 && d.d.Break(p, &i) {
			break
		}

		kst := i
		k, i = d.d.Bytes(p, i)

		vst := i
		tag, sub, end := d.d.SkipTag(p, i)
		i = end

		d.pair, _ = d.j.ConvertKey(d.pair[:0], p, kst)
		d.pair = append(d.pair, ':')
		d.pair, _ = d.j.ConvertValue(d.pair, p, vst)

		if tag == tlwire.Semantic && sub == tlog.WireLabel {
			d.ls = append(d.ls, p[kst:end])

			addComma(&d.labels)
			d.labels = append(d.labels, d.pair...)

			continue
		}

		addComma(&d.json)
		d.json = append(d.json, d.pair...)

		if tag == tlwire.Semantic && sub == tlwire.Time && string(k) == tlog.KeyTimestamp && ts == 0 {
			ts, _ = d.d.Timestamp(p, vst)
		}
	}

	addClose(&d.json)
	addClose(&d.labels)

	//

	if ts == 0 {
		ts = time.Now().UnixNano()
	}

	c := &d.cols

	c.tlog.AppendBytes(p)
	c.ls.Append(d.ls)
	c.ts.Append(proto.DateTime64(ts))

	c.json.AppendBytes(d.json)
	c.labels.AppendBytes(d.labels)

	return len(p), nil
}

func (d *Click) Query(ctx context.Context, w io.Writer, ts int64, q string) error { return nil }

func (d *Click) CreateTables(ctx context.Context) error {
	query := 0
	i := skipSpaces(schema, 0)

	for i < len(schema) {
		end := next(schema, i, ',')

		q := ch.Query{
			Body: string(schema[i:end]),
		}

		err := d.pool.Do(ctx, q)
		if err != nil {
			return errors.Wrap(err, "query %d (%d:%d)", query, i, end)
		}

		i = skipSpaces(schema, end+1)
		query++
	}

	return nil
}

func addComma(b *[]byte) {
	if len(*b) == 0 {
		*b = append(*b, '{')
	} else {
		*b = append(*b, ',')
	}
}

func addClose(b *[]byte) {
	if len(*b) != 0 {
		*b = append(*b, '}')
	}
}

func skipSpaces(b []byte, i int) int {
	for i < len(b) && (b[i] == ' ' || b[i] == '\n') {
		i++
	}

	return i
}

func next(b []byte, i int, c byte) int {
	for i < len(b) && b[i] != c {
		i++
	}

	return i
}

func csel[T any](c bool, t, e T) T {
	if c {
		return t
	} else {
		return e
	}
}
