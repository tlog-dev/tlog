// +build ignore

//nolint
package tlogdb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

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

	union []stream
)

const qstep = 4

var _ parse.Writer = &Writer{}

var tl *tlog.Logger

func NewDB(db *xrain.DB) *DB {
	return &DB{d: db}
}

func (d *DB) Messages(ls tlog.Labels, q string, it []byte, n int) (res []*parse.Message, next []byte, err error) {
	var b []byte

	err = d.d.View(func(tx *xrain.Tx) (err error) {
		mb := tx.Bucket([]byte("m"))

		var ll []stream

		if llen := len(ls); llen != 0 {
			ll = make([]stream, len(ls))

			Lm := tx.Bucket([]byte("Lm"))

			for i, l := range ls {
				lb := Lm.Bucket([]byte(l))
				if lb == nil {
					return nil
				}

				ll[i] = d.seek([]byte(l), lb, it)
			}
		}

		if q != "" {
			qb := tx.Bucket([]byte("q"))
			qq := []byte(q)
			for j := 0; j < len(qq); j += qstep {
				if e := j + qstep; e <= len(qq) {
					sub := qb.Bucket(qq[j:e])
					if sub == nil {
						return
					}

					ll = append(ll, d.seek(qq[j:e], sub, it))

					tl.Printf("add %q query filter", qq[j:j+qstep])
				} else {
					t := qb.Tree()

					var u []stream
					for st, _ := t.Seek(qq[j:], nil, nil); st != nil; st = t.Next(st) {
						key, ff := t.Key(st, nil)
						if ff != 1 {
							panic("expected bucket")
						}

						if !bytes.HasPrefix(key, qq[j:]) {
							break
						}

						sub := qb.Bucket(key)

						u = append(u, d.seek(key, sub, it))

						tl.Printf("add %q query filter from prefix %q", key, qq[j:])
					}

					ll = append(ll, union(u))
				}
			}
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

		tl.Printf("range over filters: %v", filter)
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

			res = append(res, &m)
		}

		return nil
	})

	return
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

	tl.Printf("reader next of %v last %q  from %v", s.st, s.last, tlog.Caller(1))

	if len(s.st) == 0 {
		s.st, _ = s.t.Seek(s.last, nil, s.st)
	} else {
		s.st = s.t.Next(s.st)
	}
	if s.st == nil {
		s.done = true

		return false
	}

	return true
}

func (s *reader) Key(b []byte) []byte {
	if s.done {
		return nil
	}
	if len(s.st) == 0 {
		tl.Printf("reader key   %-6v -> nil  from %v", s.st, tlog.Caller(1))
		return nil
	}

	b, _ = s.t.Key(s.st, b)

	tl.Printf("reader key   %-6v -> %q", s.st, b)

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
	val := s.s.Value(key)

	tl.Printf("picker next: %q %q", key, val)

	if len(val) == 0 {
		tl.Fatalf("empty key-value of %T %+v", s.s, s.s)
	}

	var eq bool
	s.st, eq = s.t.Seek(key, val[len(key):], s.st)

	if !eq {
		tl.Fatalf("no pick for key %q  sub: %T %v", key, s.s, s.s)
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

func (s union) Next() (r bool) {
	s.sort()

	defer func() {
		tl.Printf("union next: %v", r)
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

func (s union) Key([]byte) (k []byte) {
	s.sort()

	k = s[0].Key(nil)

	tl.Printf("union key: %q", k)

	return
}

func (s union) Value([]byte) (v []byte) {
	s.sort()

	v = s[0].Value(nil)

	tl.Printf("union val: %q", v)

	return
}

func (s union) sort() {
	sort.Slice(s, func(i, j int) bool {
		ik := s[i].Key(nil)
		jk := s[j].Key(nil)

		return bytes.Compare(ik, jk) < 0
	})
}

func (s union) String() string {
	return fmt.Sprintf("union%v", []stream(s))
}

func decodeMessage(b []byte, m *parse.Message) error {
	return json.Unmarshal(b, m)
}
