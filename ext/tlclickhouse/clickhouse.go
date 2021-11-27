package tlclickhouse

import (
	"context"
	"fmt"
	"time"

	click "github.com/nikandfor/clickhouse"
	"github.com/nikandfor/clickhouse/binary"
	"github.com/nikandfor/clickhouse/clpool"
	"github.com/nikandfor/clickhouse/dsn"
	"github.com/nikandfor/errors"
	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/convert"
	"github.com/nikandfor/tlog/low"
	"github.com/nikandfor/tlog/wire"
)

type (
	Writer struct {
		Compress bool

		table string

		pool click.ClientPool

		*convert.JSON

		b *batch

		jb, cb low.Buf
	}

	batch struct {
		cl   click.Client
		q    *click.Query
		meta click.QueryMeta

		e *binary.Encoder

		b *click.Block
	}
)

func New(addr string) (w *Writer, err error) {
	d, err := dsn.Parse(addr)
	if err != nil {
		return nil, errors.Wrap(err, "parse dsn")
	}

	w = &Writer{
		pool:     clpool.NewBinaryPool(d.Hosts[0]),
		Compress: d.Compress,
		table:    "events",
	}

	w.JSON = convert.NewJSONWriter(&w.jb)

	w.JSON.AppendNewLine = false

	w.JSON.TimeZone = time.UTC
	w.JSON.TimeFormat = "2006-01-02T15:04:05.999999999"

	w.JSON.Rename = map[convert.KeyTagSub]string{
		{Key: tlog.KeyTime, Tag: wire.Semantic, Sub: wire.Time}:               "Timestamp",
		{Key: tlog.KeyElapsed, Tag: wire.Semantic, Sub: wire.Duration}:        "Elapsed",
		{Key: tlog.KeySpan, Tag: wire.Semantic, Sub: tlog.WireID}:             "Span",
		{Key: tlog.KeyParent, Tag: wire.Semantic, Sub: tlog.WireID}:           "Parent",
		{Key: tlog.KeyLabels, Tag: wire.Semantic, Sub: tlog.WireLabels}:       "Labels",
		{Key: tlog.KeyEventKind, Tag: wire.Semantic, Sub: tlog.WireEventKind}: "EventKind",
		{Key: tlog.KeyMessage, Tag: wire.Semantic, Sub: tlog.WireMessage}:     "Message",
		{Key: tlog.KeyLogLevel, Tag: wire.Semantic, Sub: tlog.WireLogLevel}:   "LogLevel",
		{Key: tlog.KeyCaller, Tag: wire.Semantic, Sub: wire.Caller}:           "Caller",
	}

	if q := d.Query.Get("table"); q != "" {
		w.table = q
	}

	err = w.createTable(context.Background())
	if err != nil {
		return nil, errors.Wrap(err, "create table")
	}

	return w, nil
}

func (w *Writer) createTable(ctx context.Context) (err error) {
	cl, err := w.pool.Get(ctx)
	if err != nil {
		return errors.Wrap(err, "get client")
	}

	defer func() { w.pool.Put(ctx, cl, err) }()

	qq := "CREATE TABLE IF NOT EXISTS %s (" +
		"  `raw`    String          CODEC(ZSTD(9))," +
		"  `labels` Array(String)   MATERIALIZED JSONExtract(raw, 'Labels', 'Array(String)') CODEC(LZ4)," +
		"  `ts`     DateTime64(9, 'UTC') MATERIALIZED toDateTime64(JSONExtractString(raw, 'Timestamp'), 9) CODEC(Delta, ZSTD(9))," +
		"  `el`     Int64           MATERIALIZED JSONExtractInt(raw, 'Elapsed') CODEC(ZSTD(9))," +
		"  `s`      String          MATERIALIZED JSONExtractString(raw, 'Span') CODEC(ZSTD(9))," +
		"  `p`      String          MATERIALIZED JSONExtractString(raw, 'Parent') CODEC(ZSTD(9))," +
		"  `msg`    String          MATERIALIZED JSONExtractString(raw, 'Message') CODEC(ZSTD(9))," +
		"  `kind`   String          MATERIALIZED JSONExtractString(raw, 'EventKind') CODEC(ZSTD(9))," +
		"  `log_level` Int8         MATERIALIZED JSONExtract(raw, 'LogLevel', 'Int8') CODEC(ZSTD(9))," +
		"  `err`    String          MATERIALIZED JSONExtractString(raw, 'err') CODEC(ZSTD(9))," +
		"  `date`   Date            DEFAULT if(ts != 0, toDate(ts), today()) CODEC(Delta, ZSTD(9))" +
		") " +
		"ENGINE = ReplacingMergeTree() " +
		"PARTITION BY date " +
		"ORDER BY (ts, raw)"

	qq = fmt.Sprintf(qq, w.table)

	q := &click.Query{
		Query: qq,
	}

	_, err = cl.SendQuery(ctx, q)
	tlog.V("create_table").Printw("create table", "err", err)
	if err != nil {
		return errors.Wrap(err, "send query")
	}

	return nil
}

func (w *Writer) Write(p []byte) (n int, err error) {
	w.jb = w.jb[:0]

	_, err = w.JSON.Write(p)
	if err != nil {
		return 0, errors.Wrap(err, "convert to json")
	}

	b, err := w.batch()
	if err != nil {
		return 0, errors.Wrap(err, "batch")
	}

	err = w.addRow(b, w.jb)
	if err != nil {
		return 0, errors.Wrap(err, "encode string")
	}

	if b.b.Rows >= 1000000 {
		err = w.commit(b)
		if err != nil {
			return 0, errors.Wrap(err, "commit")
		}

		w.b = nil
	}

	return len(p), nil
}

func (w *Writer) Close() (err error) {
	if w.b == nil {
		return
	}

	err = w.commit(w.b)
	if err != nil {
		return errors.Wrap(err, "commit")
	}

	w.b = nil

	return nil
}

func (w *Writer) batch() (b *batch, err error) {
	if w.b != nil {
		return w.b, nil
	}

	w.b, err = w.newBatch()

	return w.b, err
}

func (w *Writer) newBatch() (b *batch, err error) {
	ctx := context.Background()

	cl, err := w.pool.Get(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "get click client")
	}

	defer func() {
		if err == nil {
			return
		}
		w.pool.Put(ctx, cl, err)
	}()

	q := &click.Query{
		Query:      fmt.Sprintf("INSERT INTO %s (`raw`) VALUES", w.table),
		Compressed: w.Compress,
	}

	meta, err := cl.SendQuery(ctx, q)
	if err != nil {
		tlog.Printw("send query", "q", q, "err", err)
		return nil, errors.Wrap(err, "send query")
	}

	if len(meta) != 1 || meta[0].Type != "String" {
		return nil, errors.New("unexpected meta: %v", meta)
	}

	b = &batch{
		cl:   cl,
		q:    q,
		meta: meta,
	}

	b.b = &click.Block{
		Rows: 0,
		Cols: meta,
	}

	w.cb = w.cb[:0]

	b.e = binary.NewEncoder(context.Background(), &w.cb)

	return b, nil
}

func (w *Writer) addRow(b *batch, row []byte) (err error) {
	err = b.e.RawString(w.jb)
	if err != nil {
		return errors.Wrap(err, "encode")
	}

	b.b.Rows++

	return
}

func (w *Writer) commit(b *batch) (err error) {
	ctx := context.Background()

	b.b.Cols[0].RawData = w.cb

	defer func() { w.pool.Put(ctx, b.cl, err) }()

	defer func() { tlog.Printw("commit", "rows", b.b.Rows, "err", err) }()

	err = b.cl.SendBlock(ctx, b.b, b.q.Compressed)
	if err != nil {
		return errors.Wrap(err, "send block")
	}

	err = b.cl.SendBlock(ctx, nil, b.q.Compressed)
	if err != nil {
		return errors.Wrap(err, "send nil block")
	}

	err = w.recvResponse(ctx, b.cl)
	if err != nil {
		return errors.Wrap(err, "recv response")
	}

	return nil
}

func (w *Writer) recvResponse(ctx context.Context, cl click.Client) (err error) {
	for {
		tp, err := cl.NextPacket(ctx)
		if err != nil {
			return errors.Wrap(err, "next packet")
		}

		switch tp {
		case click.ServerEndOfStream:
			return nil
		case click.ServerException:
			return cl.RecvException(ctx)
		default:
			return errors.New("unexpected packet: %x", tp)
		}
	}
}
