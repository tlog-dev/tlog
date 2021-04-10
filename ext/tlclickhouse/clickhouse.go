package tlclickhouse

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/nikandfor/errors"
	"github.com/nikandfor/loc"
	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/low"
)

type (
	Writer struct {
		db    *sql.DB
		tx    *sql.Tx
		s     *sql.Stmt
		count int

		table string

		d tlog.Decoder

		cols []string
		vals []interface{}

		allcols map[string]int
		dbcols  []*sql.ColumnType

		b []byte

		ls, tmpls tlog.Labels
	}
)

var ( // templates
	addCol = template.Must(template.New("add_col").Parse("ALTER TABLE {{ .table }} ADD COLUMN `{{ .name }}` {{ .type }}"))

	addRow = template.Must(template.New("add_row").Parse("INSERT INTO {{ .table }} (" +
		"{{ range $idx, $el := .cols }}{{ if ne $idx 0 }}, {{ end }}" +
		"`{{ . }}`" +
		"{{ end }}" +
		")"))
)

func New(db *sql.DB, table string) (w *Writer) {
	w = &Writer{
		db:    db,
		table: table,
	}

	return w
}

func (w *Writer) Write(p []byte) (_ int, err error) {
	defer func() {
		p := recover()
		if p == nil {
			return
		}

		tlog.Printw("panic", "panic", p)

		panic(p)
	}()

	w.d.ResetBytes(p)

	i := 0

	defer w.resetVals()

again:
	tag, els, i := w.d.Tag(i)
	if err = w.d.Err(); err != nil {
		return
	}

	if tag == tlog.Semantic && els == tlog.WireHeader {
		i = w.d.Skip(i)
		goto again
	}

	if tag != tlog.Map {
		return 0, errors.New("expected map")
	}

	err = w.prepare()
	if err != nil {
		return 0, err
	}

	err = w.begin()
	if err != nil {
		return 0, err
	}

	for el := 0; els == -1 || el < els; el++ {
		if els == -1 && w.d.Break(&i) {
			break
		}

		i, err = w.appendPair(i)
		if err != nil {
			return 0, err
		}
	}

	err = w.addRow()
	if err != nil {
		return 0, errors.Wrap(err, "add row")
	}

	if w.count == 1000 {
		err = w.commit()
		if err != nil {
			return 0, errors.Wrap(err, "commit")
		}
	}

	if w.tmpls != nil {
		w.ls = w.tmpls
		w.tmpls = nil
	}

	return len(p), nil
}

func (w *Writer) resetVals() {
	for i := range w.vals {
		col := w.dbcols[i]
		tp := col.DatabaseTypeName()

		tlog.Printw("reset vals", "i", i, "name", col.Name(), "type", tp)

		switch {
		case strings.HasPrefix(tp, "DateTime") || tp == "Date":
			w.vals[i] = time.Time{}
		case tp == "String":
			w.vals[i] = ""
		case strings.HasPrefix(tp, "Int"):
			w.vals[i] = 0
		case strings.HasPrefix(tp, "UInt"):
			w.vals[i] = uint64(0)
		case tp == "L":
			w.vals[i] = w.ls
		case strings.HasPrefix(tp, "Array"):
			w.vals[i] = []interface{}{}
		default:
			w.vals[i] = nil
		}
	}
}

func (w *Writer) appendPair(st int) (i int, err error) {
	tlog.V("pairs").Printw("append pair", "st", tlog.Hex(st))

	k, i := w.d.String(st)
	if err := w.d.Err(); err != nil {
		return 0, errors.Wrap(err, "read key")
	}

	var tp string
	var suff string

	var v interface{}

	st = i

	tag, sub, i := w.d.Tag(st)
	switch tag {
	case tlog.Int, tlog.Neg:
		tp = "Int64"
		suff = "_int"

		v, i = w.d.Int(st)
	case tlog.String, tlog.Bytes:
		tp = "String"
		suff = "_str"

		var s []byte
		s, i = w.d.String(st)

		v = string(s)
	case tlog.Array, tlog.Map:
		tp = "String"
		suff = "_json"

	case tlog.Special:
		switch sub {
		case tlog.False:
			tp = "Int8"
			suff = "_bool"

			v = 0
		case tlog.True:
			tp = "Int8"
			suff = "_bool"

			v = 1
		case tlog.Null:
			// do not add
			return i, nil
		case tlog.Undefined:
			// do not add
			return i, nil
		case tlog.Float64, tlog.Float32, tlog.FloatInt8:
			tp = "Float64"
			suff = "_flt"

			v, i = w.d.Float(st)
		default:
			panic(sub)
		}
	case tlog.Semantic:
		switch sub {
		case tlog.WireLabels:
			w.tmpls, i = w.d.Labels(st)
			if err = w.d.Err(); err != nil {
				return i, errors.Wrap(err, "read lables")
			}

			v = w.tmpls
		case tlog.WireTime:
			tp = "DateTime64"
			suff = "_time"

			var ts tlog.Timestamp
			ts, i = w.d.Time(st)
			if err = w.d.Err(); err != nil {
				return i, errors.Wrap(err, "read time")
			}

			v = ts.Time()
		case tlog.WireLocation:
			tp = "String"
			suff = "_loc"

			var pc loc.PC
			pc, _, i = w.d.Location(st)
			if err = w.d.Err(); err != nil {
				return i, errors.Wrap(err, "read time")
			}

			v = fmt.Sprintf("%+v", pc)
		case tlog.WireMessage:
			tp = "String"
			suff = "_msg"

			var s []byte
			s, i = w.d.String(i)
			if err = w.d.Err(); err != nil {
				return i, errors.Wrap(err, "read time")
			}

			v = string(s)
		case tlog.WireID:
			tp = "String"
			suff = "_id"

			var id tlog.ID
			id, i = w.d.ID(st)
			if err = w.d.Err(); err != nil {
				return i, errors.Wrap(err, "read id")
			}

			v = id.FullString()
		case tlog.WireError:
			tp = "String"
			suff = "_err"

			var s []byte
			s, i = w.d.String(i)
			if err = w.d.Err(); err != nil {
				return i, errors.Wrap(err, "read error")
			}

			v = string(s)
		case tlog.WireHex:
			tp = "UInt64"
			suff = "_hex"

			var s int64
			s, i = w.d.Int(i)
			if err = w.d.Err(); err != nil {
				return i, errors.Wrap(err, "read int")
			}

			v = uint64(s)
		case tlog.WireEventType:
			tp = "String"
			suff = "_evtype"

			var s []byte
			s, i = w.d.String(i)
			if err = w.d.Err(); err != nil {
				return i, errors.Wrap(err, "read string")
			}

			v = string(s)
		default:
			tp = "String"
			suff += fmt.Sprintf("_%02x", sub)

			w.b, i = w.appendJSONValue(w.b[:0], i)

			v = string(w.b)
		}
	}

	col := string(k) + suff
	coln, ok := w.allcols[col]

	if !ok {
		err := w.addColumn(tp, col)
		if err != nil {
			return 0, errors.Wrap(err, "add column")
		}

		coln = w.allcols[col]
	}

	tlog.V("column").Printw("set column value", "name", col, "coln", coln, "val", v, "oldval", w.vals[coln])

	if w.vals[coln] != nil {
		// duplicated column
		return i, nil
	}

	w.vals[coln] = v

	return i, nil
}

func (w *Writer) appendJSONValue(b []byte, st int) (_ []byte, i int) {
	tag, sub, i := w.d.Tag(st)
	if w.d.Err() != nil {
		return
	}

	var v int64
	var s []byte
	var f float64

	switch tag {
	case tlog.Int:
		v, i = w.d.Int(st)

		b = strconv.AppendUint(b, uint64(v), 10)
	case tlog.Neg:
		v, i = w.d.Int(st)

		b = strconv.AppendInt(b, v, 10)
	case tlog.Bytes:
		s, i = w.d.String(st)

		b = append(b, '"')

		m := base64.StdEncoding.EncodedLen(len(s))
		d := len(b)

		for cap(b)-d < m {
			b = append(b[:cap(b)], 0, 0, 0, 0)
		}

		b = b[:d+m]

		base64.StdEncoding.Encode(b[d:], s)

		b = append(b, '"')
	case tlog.String:
		s, i = w.d.String(st)

		b = low.AppendQuote(b, low.UnsafeBytesToString(s))

	case tlog.Array:
		b = append(b, '[')

		for el := 0; sub == -1 || el < sub; el++ {
			if sub == -1 && w.d.Break(&i) {
				break
			}

			if el != 0 {
				b = append(b, ',')
			}

			b, i = w.appendJSONValue(b, i)
		}

		b = append(b, ']')
	case tlog.Map:
		b = append(b, '{')

		for el := 0; sub == -1 || el < sub; el++ {
			if sub == -1 && w.d.Break(&i) {
				break
			}

			if el != 0 {
				b = append(b, ',')
			}

			b, i = w.appendJSONValue(b, i)

			b = append(b, ':')

			b, i = w.appendJSONValue(b, i)
		}

		b = append(b, '}')
	case tlog.Semantic:
		b, i = w.appendJSONValue(b, i)
	case tlog.Special:
		switch sub {
		case tlog.False:
			b = append(b, "false"...)
		case tlog.True:
			b = append(b, "true"...)
		case tlog.Null, tlog.Undefined:
			b = append(b, "null"...)
		case tlog.Float64, tlog.Float32, tlog.Float16, tlog.FloatInt8:
			f, i = w.d.Float(st)

			b = strconv.AppendFloat(b, f, 'f', -1, 64)
		default:
			panic(sub)
		}
	}

	return b, i
}

func (w *Writer) begin() (err error) {
	if w.tx != nil {
		return nil
	}

	w.tx, err = w.db.Begin()
	if err != nil {
		return errors.Wrap(err, "begin")
	}

	var buf bytes.Buffer
	err = addRow.Execute(&buf, map[string]interface{}{
		"table": w.table,
		"cols":  w.cols,
	})
	if err != nil {
		return errors.Wrap(err, "qeury template")
	}

	q := buf.String()

	w.s, err = w.tx.Prepare(q)
	tlog.V("query").PrintwDepth(1, "prepare", "q", q, "err", err)
	if err != nil {
		return errors.Wrap(err, "prepare")
	}

	return nil
}

func (w *Writer) prepare() (err error) {
	if w.allcols != nil {
		return nil
	}

	w.allcols = make(map[string]int)

	q := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		t_time DateTime64(9) NOT NULL,
		date_ Date DEFAULT toDate(t_time),
		L Array(String)
	) ENGINE = MergeTree() PARTITION BY date_ ORDER BY (t_time, L)`, w.table)

	_, err = w.db.Exec(q)
	tlog.V("query").Printw("create table", "q", q, "err", err)
	if err != nil {
		return errors.Wrap(err, "create table")
	}

	q = fmt.Sprintf(`SELECT * FROM %s LIMIT 1`, w.table)

	rows, err := w.db.Query(q)
	tlog.V("query").Printw("get columns", "q", q, "err", err)
	if err != nil {
		return errors.Wrap(err, "get columns")
	}

	w.dbcols, err = rows.ColumnTypes()
	if err != nil {
		return errors.Wrap(err, "get db column types")
	}

	cols, err := rows.Columns()
	if err != nil {
		return errors.Wrap(err, "get column names")
	}

	tlog.Printw("cols", "cols", cols)

	p := -1
	for i, c := range cols {
		if c == "date_" {
			p = i
			break
		}
	}

	if p != -1 {
		l := len(cols) - 1

		copy(cols[p:], cols[p+1:])
		copy(w.dbcols[p:], w.dbcols[p+1:])

		cols = cols[:l]
		w.dbcols = w.dbcols[:l]
	}

	for i, c := range cols {
		w.allcols[c] = i
		w.cols = append(w.cols, c)
	}

	w.vals = make([]interface{}, len(w.cols))

	w.resetVals()

	return nil
}

func (w *Writer) addColumn(tp, name string) (err error) {
	err = w.commit()
	if err != nil {
		return errors.Wrap(err, "commit")
	}

	var buf bytes.Buffer

	err = addCol.Execute(&buf, map[string]interface{}{
		"table": w.table,
		"name":  name,
		"type":  tp,
	})
	if err != nil {
		return errors.Wrap(err, "qeury template")
	}

	q := buf.String()

	_, err = w.db.Query(q)
	tlog.V("query").Printw("query", "q", q, "err", err)
	if err != nil {
		return errors.Wrap(err, "prepare")
	}

	w.allcols[name] = len(w.cols)
	w.cols = append(w.cols, name)
	w.vals = append(w.vals, nil)

	err = w.begin()
	if err != nil {
		return errors.Wrap(err, "begin")
	}

	w.resetVals()

	return nil
}

func (w *Writer) addRow() (err error) {
	_, err = w.s.Exec(w.vals...)
	tlog.V("query").PrintwDepth(1, "add row", "args", w.vals, "err", err)
	if err != nil {
		return errors.Wrap(err, "exec")
	}

	w.count++

	return nil
}

func (w *Writer) commit() (err error) {
	if w.tx == nil {
		return nil
	}

	if w.count != 0 {
		err = w.tx.Commit()
		err = errors.Wrap(err, "commit")
	} else {
		err = w.tx.Rollback()
		err = errors.Wrap(err, "rollback")
	}

	w.tx = nil
	w.s = nil

	if err != nil {
		return err
	}

	return nil
}

func (w *Writer) Close() (err error) {
	err = w.commit()
	if err != nil {
		return errors.Wrap(err, "commit")
	}

	err = w.db.Close()
	if err != nil {
		return errors.Wrap(err, "close db")
	}

	return nil
}
