//nolint
package tlogdb

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/nikandfor/xrain"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/parse"
)

/*
	DB structure:

	-> means bucket
	=> means key-value pair

	ts ::= 8 byte timestamp
	message_ctx ::= crc32(Labels | span_id)

	lh -> <hash> => <location>
	ls -> <hash> => <span_ts> => <span_id>
	lm -> <hash> => <message_ts> => <message_ctx>

	Ls -> <k=v> -> <span_ts> => <span_id>
	Lm -> <k=v> -> <message_ts> => <message_ctx>

	sL -> <span_id> => <labels>

	m -> <ts> => <message_ctx> | <message>
	s -> <ts> => <span_start>
	f -> <ts> => <span_finish>

	i -> <span_id> => <span_ts>
	p -> <span_id> => <parent_span_id>
	c -> <span_id> -> <child_span_ts> => <child_span_id>

	sm -> <span_id> -> <message_ts> => <message_ctx>

	q -> <pref> -> <message_ts> => <message_ctx>
*/

type (
	ID     = tlog.ID
	Labels = tlog.Labels

	DB struct {
		d *xrain.DB
	}

	Event struct {
		*parse.SpanStart
		*parse.SpanFinish
		*parse.Message
		*parse.Metric
	}

	stream interface {
		fmt.Stringer

		Next() bool
		Key([]byte) []byte
		Value([]byte) []byte
	}

	reader struct {
		name []byte
		t    *xrain.LayoutShortcut
		st   xrain.Stack

		last []byte
		done bool
	}

	picker struct {
		name []byte
		t    *xrain.LayoutShortcut
		s    stream

		st xrain.Stack

		done bool
	}

	intersector []stream

	merge []stream
)

const qstep, qMaxPrefix = 4, 16

var tl *tlog.Logger

var (
	ErrTooShortQuery = errors.New("too short query")
)

func NewDB(db *xrain.DB) *DB {
	return &DB{d: db}
}

func (d *DB) Messages(ls []Labels, sid ID, q string, it []byte, n int) (res []*parse.Message, next []byte, err error) {
	var b []byte

	err = d.d.View(func(tx *xrain.Tx) (err error) {
		mb := tx.Bucket([]byte("m"))

		var ll []stream

		if sid != (ID{}) {
			b := tx.Bucket([]byte("sm"))
			ll = append(ll, d.seek([]byte("sm"), b, it))
		}

		if f, err := d.labels(tx, []byte("Lm"), ls, it); err != nil {
			return err
		} else if f != nil {
			ll = append(ll, f)
		}

		if f, err := d.query(tx, []byte(q), it); err != nil {
			return err
		} else if f != nil {
			ll = append(ll, f)
		}

		var filter stream
		switch len(ll) {
		case 0:
			filter = d.seek([]byte("m"), mb, it)
		case 1:
			filter = d.picker([]byte("m"), mb, ll[0])
		default:
			filter = intersector(ll)
			filter = d.picker([]byte("m"), mb, filter)
		}

		tl.Printf("range over filter: %v  it %q", filter, it)
		for filter.Next() {
			if len(res) == n {
				next = filter.Key(b[:0])
				break
			}

			b = filter.Value(b[:0])

			var m parse.Message
			err = decodeMessage(b, &m)
			if err != nil {
				return err
			}

			if len(q) != 0 && !strings.Contains(m.Text, q) {
				tl.Printf("skip message by query %q: %q", q, m.Text)
				continue
			}

			res = append(res, &m)
		}

		return nil
	})

	return
}

func (d *DB) labels(tx *xrain.Tx, bn []byte, ls []tlog.Labels, it []byte) (f stream, err error) {
	if len(ls) == 0 {
		return
	}

	L := tx.Bucket(bn)
	if L == nil {
		return
	}

	or := make([]stream, 0, len(ls))

	for _, ls := range ls {
		if len(ls) == 0 {
			continue
		}

		and := make([]stream, 0, len(ls))

		for _, l := range ls {
			lb := L.Bucket([]byte(l))
			if lb == nil {
				return
			}

			and = append(and, d.seek([]byte(l), lb, it))
		}

		switch len(and) {
		case 0:
		case 1:
			or = append(or, and[0])
		default:
			or = append(or, intersector(and))
		}
	}

	switch len(or) {
	case 0:
	case 1:
		f = or[0]
	default:
		f = merge(or)
	}

	return
}

func (d *DB) query(tx *xrain.Tx, q, it []byte) (_ stream, err error) {
	if len(q) == 0 {
		return
	}
	if len(q) < qstep {
		return d.queryShort(tx, q, it)
	}

	qb := tx.Bucket([]byte("q"))
	i := 0

	for i+qstep <= len(q) {
		sub := qb.Bucket(q[i : i+qstep])
		if sub == nil {
			return
		}

		qb = sub
		i += qstep
	}

	return d.seek(q[:i], qb, it), nil
}

func (d *DB) queryShort(tx *xrain.Tx, q, it []byte) (_ stream, err error) {
	var buf, key []byte
	qb := tx.Bucket([]byte("q"))
	t := qb.Tree()

	var ll []stream

	for st, _ := t.Seek(q, nil, nil); st != nil; st = t.Next(st) {
		k, ff := t.Key(st, buf)
		if ff != 1 {
			panic(ff)
		}
		key, buf = k[len(buf):], k

		if !bytes.HasPrefix(key, q) {
			break
		}

		tl.Printf("query merge: bucket %q <- q %q  st %v", key, q, st)

		sub := qb.Bucket(key)

		ll = append(ll, d.seek(key, sub, it))
	}

	switch len(ll) {
	case 0:
		return
	case 1:
		return ll[0], nil
	}

	return merge(ll), nil
}

func (d *DB) seek(name []byte, b *xrain.SimpleBucket, it []byte) (s *reader) {
	return &reader{
		name: name,
		t:    b.Tree(),
		last: it,
	}
}

func (s *reader) Next() bool {
	if s.done {
		return false
	}

	tl.Printf("reader %6q  next %-6v last %q  from %v", s.name, s.st, s.last, tlog.Caller(1))

next:
	if len(s.st) == 0 {
		s.st, _ = s.t.Seek(s.last, nil, s.st)
	} else {
		s.st = s.t.Next(s.st)
	}

	if s.st == nil {
		s.done = true

		return false
	}

	if ff := s.t.Flags(s.st); ff != 0 {
		goto next
	}

	return true
}

func (s *reader) Key(b []byte) []byte {
	if s.done {
		return nil
	}
	if len(s.st) == 0 {
		tl.Printf("reader %6q  key  %-6v -> nil  from %v", s.name, s.st, tlog.Caller(1))
		return nil
	}

	b, _ = s.t.Key(s.st, b)

	tl.Printf("reader %6q  key  %-6v -> %q", s.name, s.st, b)

	return b
}

func (s *reader) Value(b []byte) []byte {
	if s.done {
		return nil
	}
	if len(s.st) == 0 {
		return nil
	}

	return s.t.Value(s.st, b)
}

func (s *reader) String() string {
	return fmt.Sprintf("reader{%q done:%5v st:%v}", s.name, s.done, s.st)
}

func (d *DB) picker(name []byte, b *xrain.SimpleBucket, r stream) (s *picker) {
	return &picker{
		name: name,
		t:    b.Tree(),
		s:    r,
	}
}

func (s *picker) Next() bool {
	ok := s.s.Next()
	if !ok {
		return false
	}

	key := s.s.Key(nil)
	val := s.s.Value(key)[len(key):]

	tl.Printf("picker next. at %q pick: [%q => %q]", s.name, key, val)

	if len(key) == 0 {
		tl.Fatalf("empty key-value of %T %+v", s.s, s.s)
	}

	var eq bool
	s.st, eq = s.t.Seek(key, val, s.st)

	if !eq {
		tl.Fatalf("FATAL: no pick for key %q  sub: %T %v", key, s.s, s.s)
	}

	return true
}

func (s *picker) Key(b []byte) []byte {
	if len(s.st) == 0 {
		return nil
	}
	b, _ = s.t.Key(s.st, b)
	return b
}

func (s *picker) Value(b []byte) []byte {
	if len(s.st) == 0 {
		return nil
	}
	return s.t.Value(s.st, b)
}

func (s *picker) String() string {
	return fmt.Sprintf("picker{%q done:%5v st:%v s:%v}", s.name, s.done, s.st, s.s.String())
}

func (s intersector) Next() bool {
	tl.Printf("intersector next")

	return s.even(true)
}

func (s intersector) Key([]byte) []byte {
	if !s.even(false) {
		return nil
	}

	return s[0].Key(nil)
}

func (s intersector) Value([]byte) []byte {
	if !s.even(false) {
		return nil
	}

	return s[0].Value(nil)
}

func (s intersector) sort() {
	sort.Slice(s, func(i, j int) bool {
		ik := s[i].Key(nil)
		jk := s[j].Key(nil)

		return bytes.Compare(ik, jk) < 0
	})
}

func (s intersector) even(move bool) bool {
again:
	var zk []byte = nil

	for i := range s {
		k := s[i].Key(nil)

		if i == 0 {
			zk = k
		}

		if i == 0 || bytes.Equal(k, zk) {
			continue
		}

		if move {
			return false
		}

		for j := 0; j < i; j++ {
			if !s[j].Next() {
				return false
			}
		}
	}

	if !move {
		return true
	}

	if !s[0].Next() {
		return false
	}

	move = false

	goto again
}

func (s intersector) String() string {
	return fmt.Sprintf("intersector%v", []stream(s))
}

func (s merge) Next() (r bool) {
	s.sort()

	defer func() {
		tl.Printf("merge next: %v", r)
	}()

	var zk []byte

	for i, s := range s {
		k := s.Key(nil)

		if i == 0 {
			zk = k
		}

		if i == 0 || bytes.Equal(k, zk) {
			if !s.Next() {
				return false
			}
		}

		break
	}

	return len(s) != 0
}

func (s merge) Key([]byte) (k []byte) {
	s.sort()

	k = s[0].Key(nil)

	tl.Printf("merge key: %q", k)

	return
}

func (s merge) Value([]byte) (v []byte) {
	s.sort()

	v = s[0].Value(nil)

	tl.Printf("merge val: %q", v)

	return
}

func (s merge) sort() {
	sort.Slice(s, func(i, j int) bool {
		ik := s[i].Key(nil)
		jk := s[j].Key(nil)

		return bytes.Compare(ik, jk) < 0
	})
}

func (s merge) String() string {
	return fmt.Sprintf("merge%v", []stream(s))
}

func decodeMessage(b []byte, m *parse.Message) error {
	return json.Unmarshal(b, m)
}
