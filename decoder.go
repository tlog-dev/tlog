package tlog

import (
	"io"
	"math"

	"github.com/nikandfor/errors"
	"github.com/nikandfor/loc"
	"github.com/nikandfor/tlog/low"
)

type (
	Decoder struct {
		io.Reader

		err error

		b         []byte
		ref, keep int64
		//	pos       int64
	}

	Dumper struct {
		io.Writer

		d Decoder

		NoGlobalOffset bool

		ref int64
		b   low.Buf
	}
)

func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{
		Reader: r,
		keep:   -1,
	}
}

func NewDecoderBytes(b []byte) *Decoder {
	return &Decoder{
		b:    b,
		keep: -1,
	}
}

func (d *Decoder) Reset(r io.Reader) {
	d.Reader = r
	d.ResetBytes(d.b[:0])
}

func (d *Decoder) ResetBytes(b []byte) {
	d.b = b
	d.ref = 0
	d.keep = -1
	d.err = nil
}

func (d *Decoder) Keep(st int64) {
	d.keep = st
}

func (d *Decoder) Bytes(st, end int64) []byte {
	if !d.more(st, end) {
		return nil
	}

	return d.b[st-d.ref : end-d.ref]
}

func (d *Decoder) Skip(st int64) (i int64) {
	tag, sub, i := d.Tag(st)
	if d.err != nil {
		return
	}

	switch tag {
	case Int, Neg:
		_, i = d.Int(st)
	case Bytes, String:
		_, i = d.String(st)
	case Array, Map:
		for el := 0; sub == -1 || el < sub; el++ {
			if sub == -1 && d.Break(&i) {
				break
			}

			i = d.Skip(i)

			if tag == Map {
				i = d.Skip(i)
			}
		}
	case Semantic:
		i = d.Skip(i)
	case Special:
		switch sub {
		case False, True, Null, Undefined:
		case FloatInt8, Float16, Float32, Float64:
			_, i = d.Float(st)
		default:
			d.newErr(st, "unsupported special")
		}
	default:
		panic(tag)
	}

	return
}

func (d *Decoder) ID(st int64) (id ID, i int64) {
	tag, sub, i := d.Tag(st)
	if d.err != nil {
		return
	}

	if tag != Semantic || sub != WireID {
		d.newErr(st, "expected ID")
		return
	}

	s, i := d.String(i)
	copy(id[:], s)

	return
}

func (d *Decoder) Caller(st int64) (pc loc.PC, pcs loc.PCs, i int64) {
	tag, sub, i := d.Tag(st)
	if d.err != nil {
		return
	}

	if tag != Semantic || sub != WireCaller {
		d.newErr(st, "expected location")
		return
	}

	st = i
	tag, sub, i = d.Tag(i)
	if d.err != nil {
		return
	}

	if tag == Array {
		pcs, i = d.locationStack(st)

		return
	}

	pc, i = d.location(st)

	return

}

func (d *Decoder) location(st int64) (pc loc.PC, i int64) {
	tag, sub, i := d.Tag(st)
	if d.err != nil {
		return
	}

	var v int64

	if tag == Int {
		v, i = d.Int(st)

		return loc.PC(v), i
	}

	if tag != Map {
		d.newErr(st, "expected location (map or int)")
		return
	}

	var name, file string
	var line int

	var k []byte
	for el := 0; sub == -1 || el < sub; el++ {
		if sub == -1 && d.Break(&i) {
			break
		}

		k, i = d.String(i)
		if d.err != nil {
			return
		}
		if len(k) == 0 {
			d.newErr(st, "location map: empty key")
		}

		switch k[0] {
		case 'p':
			v, i = d.Int(i)
			pc = loc.PC(v)
		case 'n':
			k, i = d.String(i)
			name = string(k)
		case 'f':
			k, i = d.String(i)
			file = string(k)
		case 'l':
			v, i = d.Int(i)
			line = int(v)
		default:
			i = d.Skip(i)
		}
	}

	pc.SetCache(name, file, line)

	return
}

func (d *Decoder) locationStack(st int64) (pcs loc.PCs, i int64) {
	tag, els, i := d.Tag(st)
	if d.err != nil {
		return
	}

	if tag != Array {
		d.newErr(st, "expected location (array)")
		return
	}

	if els != -1 {
		pcs = make(loc.PCs, 0, els)
	}

	var pc loc.PC
	for el := 0; els == -1 || el < els; el++ {
		pc, i = d.location(i)

		pcs = append(pcs, pc)
	}

	return
}

func (d *Decoder) Labels(st int64) (ls Labels, i int64) {
	tag, sub, i := d.Tag(st)
	if d.err != nil {
		return
	}

	if tag != Semantic || sub != WireLabels {
		d.newErr(st, "expected labels")
		return
	}

	tag, sub, i = d.Tag(i)
	if d.err != nil {
		return
	}

	if tag != Array {
		d.newErr(st, "expected labels (array)")
		return
	}

	var s []byte
	for el := 0; sub == -1 || el < sub; el++ {
		if sub == -1 && d.Break(&i) {
			break
		}

		s, i = d.String(i)
		if d.err != nil {
			return nil, i
		}

		ls = append(ls, string(s))
	}

	return
}

func (d *Decoder) Time(st int64) (ts Timestamp, i int64) {
	tag, sub, i := d.Tag(st)
	if d.err != nil {
		return
	}

	if tag != Semantic || sub != WireTime {
		d.newErr(st, "expected time")
		return
	}

	v, i := d.Int(i)

	return Timestamp(v), i
}

func (d *Decoder) LogLevel(st int64) (lv LogLevel, i int64) {
	tag, sub, i := d.Tag(st)
	if d.err != nil {
		return
	}

	if tag != Semantic || sub != WireLogLevel {
		d.newErr(st, "expected time")
		return
	}

	v, i := d.Int(i)

	return LogLevel(v), i
}

func (d *Decoder) EventType(st int64) (e EventType, i int64) {
	tag, sub, i := d.Tag(st)
	if d.err != nil {
		return
	}

	if tag != Semantic || sub != WireEventType {
		d.newErr(st, "expected event type")
		return
	}

	s, i := d.String(i)
	e = EventType(s)

	return
}

func (d *Decoder) String(st int64) (s []byte, i int64) {
	tag, l, i := d.Tag(st)

	if tag != String && tag != Bytes {
		d.newErr(st, "wanted string/bytes")
		return
	}

	if !d.more(st, i+int64(l)) {
		return
	}

	s = d.b[i-d.ref : i-d.ref+int64(l)]
	i += int64(l)

	return s, i
}

func (d *Decoder) Float(st int64) (v float64, i int64) {
	tag, sub, i := d.Tag(st)

	if tag != Special {
		d.newErr(st, "wanted float")
		return
	}

	switch sub {
	case FloatInt8:
		if !d.more(st, i+1) {
			return
		}

		q := int8(d.b[i-d.ref])
		i++

		return float64(q), i
	case Float32:
		if !d.more(st, i+4) {
			return
		}

		q := uint32(d.b[i-d.ref])<<24 | uint32(d.b[i+1-d.ref])<<16 | uint32(d.b[i+2-d.ref])<<8 | uint32(d.b[i+3-d.ref])
		i += 4

		return float64(math.Float32frombits(q)), i
	case Float64:
		if !d.more(st, i+8) {
			return
		}

		q := uint64(d.b[i-d.ref])<<56 | uint64(d.b[i+1-d.ref])<<48 | uint64(d.b[i+2-d.ref])<<40 | uint64(d.b[i+3-d.ref])<<32 |
			uint64(d.b[i+4-d.ref])<<24 | uint64(d.b[i+5-d.ref])<<16 | uint64(d.b[i+6-d.ref])<<8 | uint64(d.b[i+7-d.ref])
		i += 8

		return math.Float64frombits(q), i
	default:
		d.newErr(st, "unsupported float specials: %x", sub)
	}

	return
}

func (d *Decoder) Int(st int64) (v int64, i int64) {
	i = st

	if !d.more(i, i+1) {
		return -1, st
	}

	tag := int(d.b[i-d.ref] & TypeMask)
	if tag != Int && tag != Neg {
		d.newErr(st, "expected int: got %x", tag)
		return
	}

	v = int64(d.b[i-d.ref] & TypeDetMask)
	i++

	switch {
	case v < Len1:
		// v = v
	case v == Len1:
		if !d.more(st, i+1) {
			return -1, i
		}

		v = int64(d.b[i-d.ref])
		i++
	case v == Len2:
		if !d.more(st, i+2) {
			return -1, i
		}

		v = int64(d.b[i-d.ref])<<8 | int64(d.b[i+1-d.ref])
		i += 2
	case v == Len4:
		if !d.more(st, i+4) {
			return -1, i
		}

		v = int64(d.b[i-d.ref])<<24 | int64(d.b[i+1-d.ref])<<16 | int64(d.b[i+2-d.ref])<<8 | int64(d.b[i+3-d.ref])
		i += 4
	case v == Len8:
		if !d.more(st, i+8) {
			return -1, i
		}

		v = int64(d.b[i-d.ref])<<56 | int64(d.b[i+1-d.ref])<<48 | int64(d.b[i+2-d.ref])<<40 | int64(d.b[i+3-d.ref])<<32 |
			int64(d.b[i+4-d.ref])<<24 | int64(d.b[i+5-d.ref])<<16 | int64(d.b[i+6-d.ref])<<8 | int64(d.b[i+7-d.ref])
		i += 8
	default:
		d.newErr(st, "unsupported int len: %v", v)
	}

	if tag == Neg {
		v = -v
	}

	return
}

func (d *Decoder) Tag(st int64) (tag, sub int, i int64) {
	i = st

	//	defer func() {
	//		fmt.Fprintf(Stderr, "Tag %3x -> %3x : %2x %2x  at %#v\n", st, i, tag, sub, loc.Callers(2, 4))
	//	}()

	if !d.more(st, i+1) {
		return -1, -1, st
	}

	tag = int(d.b[i-d.ref] & TypeMask)
	sub = int(d.b[i-d.ref] & TypeDetMask)
	i++

	if tag == Special {
		return
	}

	switch {
	case sub < Len1:
		// sub = sub
	case sub == Len1:
		if !d.more(st, i+1) {
			return -1, -1, st
		}

		sub = int(d.b[i-d.ref])
		i++
	case sub == Len2:
		if !d.more(st, i+2) {
			return -1, -1, st
		}

		sub = int(d.b[i-d.ref])<<8 | int(d.b[i+1-d.ref])
		i += 2
	case sub == Len4:
		if !d.more(st, i+4) {
			return -1, -1, st
		}

		sub = int(d.b[i-d.ref])<<24 | int(d.b[i+1-d.ref])<<16 | int(d.b[i+2-d.ref])<<8 | int(d.b[i+3-d.ref])
		i += 4
	case sub == Len8:
		if !d.more(st, i+8) {
			return -1, -1, st
		}

		sub = int(d.b[i-d.ref])<<56 | int(d.b[i+1-d.ref])<<48 | int(d.b[i+2-d.ref])<<40 | int(d.b[i+3-d.ref])<<32 |
			int(d.b[i+4-d.ref])<<24 | int(d.b[i+5-d.ref])<<16 | int(d.b[i+6-d.ref])<<8 | int(d.b[i+7-d.ref])
		i += 8
	case sub == LenBreak:
		sub = -1
	default:
		d.newErr(st, "unsupported int len: %v", sub)
	}

	return
}

func (d *Decoder) Break(i *int64) bool {
	if !d.more(*i, *i+1) {
		return true
	}

	if d.b[*i-d.ref] != Special|Break {
		return false
	}

	(*i)++

	return true
}

func (d *Decoder) more(st, end int64) (res bool) {
	if d.err != nil {
		return false
	}

	if int(end-d.ref) <= len(d.b) {
		return true
	}

	//	defer func(ref int64, b []byte) {
	//		fmt.Fprintf(Stderr, "more st %3x - %3x res %v  ref %3x <- %3x len %3x/%3x <- %3x/%3x  at %#v\n", st, end, res, d.ref, ref, len(d.b), cap(d.b), len(b), cap(b), loc.Callers(2, 4))
	//	}(d.ref, d.b)

	if d.Reader == nil {
		d.wrapErr(st, io.ErrUnexpectedEOF, "short buffer, no reader")
		return false
	}

	// [0] already-used [st] not-used [len(d.b)] free-space [cap(d.b)]
	// d.ref is start of d.b position in stream
	// d.keep is start of protected zone

	keep := st
	if d.keep != -1 && d.keep < keep {
		keep = d.keep
	}

	keep -= d.ref

	if keep != 0 {
		n := copy(d.b, d.b[keep:])
		d.b = d.b[:n]
		d.ref += keep
		keep = 0
	}

	read := len(d.b)

	d.grow(int(end - d.ref))

more:
	n, err := d.Reader.Read(d.b[read:])
	//	println("more", d.keep, st, end, "read", read, cap(d.b), n)
	read += n

	if err != nil {
		d.err = err
		d.b = d.b[:read]
		return false
	}

	if end <= d.ref+int64(read) {
		d.b = d.b[:read]
		return true
	}

	goto more
}

func (d *Decoder) grow(l int) {
	n := cap(d.b)

	for n < l {
		switch {
		case n == 0:
			n = 1
		case n < 1024:
			n *= 2
		default:
			n = n * 5 / 4
		}
	}

	if cap(d.b) < n {
		b := make([]byte, n)
		copy(b, d.b)
		d.b = b
	}

	d.b = d.b[:cap(d.b)]
}

func (d *Decoder) wrapErr(i int64, err error, f string, args ...interface{}) {
	if d.err != nil {
		return
	}

	d.err = errors.WrapDepth(err, 1, f, args...)
	d.err = errors.WrapNoLoc(d.err, "(pos %x)", i)
}

func (d *Decoder) newErr(i int64, f string, args ...interface{}) {
	if d.err != nil {
		return
	}

	d.err = errors.NewDepth(1, f, args...)
	d.err = errors.WrapNoLoc(d.err, "(pos %x)", i)
}

func (d *Decoder) Err() error {
	return d.err
}

func (d *Decoder) ResetErr() { d.err = nil }

func NewDumper(w io.Writer) *Dumper {
	return &Dumper{
		Writer: w,
	}
}

func (w *Dumper) Write(p []byte) (int, error) {
	w.d.ResetBytes(p)
	w.b = w.b[:0]

	var i int64
	for int(i) < len(p) {
		i = w.dump(i, 0)
	}

	if w.d.err != nil {
		return 0, w.d.Err()
	}

	w.ref += i

	if w.Writer != nil {
		return w.Writer.Write(w.b)
	}

	return len(p), nil
}

func (w *Dumper) dump(st int64, d int) (i int64) {
	tag, sub, i := w.d.Tag(st)
	if w.d.err != nil {
		return
	}

	if !w.NoGlobalOffset {
		w.b = low.AppendPrintf(w.b, "%8x  ", w.ref+st)
	}

	w.b = low.AppendPrintf(w.b, "%4x  %s% x  -  ", st, low.Spaces[:d*2], w.d.b[st:i])

	switch tag {
	case Int, Neg:
		var v int64
		v, i = w.d.Int(st)

		w.b = low.AppendPrintf(w.b, "int %10v\n", v)
	case Bytes, String:
		var s []byte
		s, i = w.d.String(st)

		if tag == Bytes {
			w.b = low.AppendPrintf(w.b, "% x\n", s)
		} else {
			w.b = low.AppendPrintf(w.b, "%q\n", s)
		}
	case Array, Map:
		tg := "array"
		if tag == Map {
			tg = "map"
		}

		w.b = low.AppendPrintf(w.b, "%v: len %v\n", tg, sub)

		for el := 0; sub == -1 || el < sub; el++ {
			if sub == -1 && w.d.Break(&i) {
				i = w.dump(i-1, d+1)
				break
			}

			i = w.dump(i, d+1)

			if tag == Map {
				i = w.dump(i, d+1)
			}
		}
	case Semantic:
		w.b = low.AppendPrintf(w.b, "semantic %2x\n", sub)

		i = w.dump(i, d+1)
	case Special:
		switch sub {
		case False:
			w.b = low.AppendPrintf(w.b, "false\n")
		case True:
			w.b = low.AppendPrintf(w.b, "true\n")
		case Null:
			w.b = low.AppendPrintf(w.b, "null\n")
		case Undefined:
			w.b = low.AppendPrintf(w.b, "undefined\n")
		case FloatInt8, Float16, Float32, Float64:
			var f float64

			f, i = w.d.Float(st)

			w.b = low.AppendPrintf(w.b, "%v\n", f)
		case Break:
			w.b = low.AppendPrintf(w.b, "break\n")
		default:
			w.b = low.AppendPrintf(w.b, "special: %x\n", sub)
			w.d.newErr(st, "unsupported special")
		}
	default:
		w.d.newErr(st, "read impossible tag")
	}

	return
}

func Dump(p []byte) string {
	var d Dumper
	d.NoGlobalOffset = true

	d.Write(p)

	return string(d.b)
}

func (d *Decoder) MessageEventType(i int64) (e EventType) {
	tag, els, i := d.Tag(i)

	if tag != Map {
		return
	}

	var k []byte
	var sub int
	for el := 0; els == -1 || el < els; el++ {
		if els == -1 && d.Break(&i) {
			break
		}

		k, i = d.String(i)

		tag, sub, _ = d.Tag(i)

		if tag == Semantic && sub == WireEventType && string(k) == KeyEventType {
			e, _ = d.EventType(i)

			return e
		}

		i = d.Skip(i)
	}

	return
}
