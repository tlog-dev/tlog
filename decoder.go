package tlog

import (
	"fmt"
	"io"
	"math"

	"github.com/nikandfor/loc"
	"github.com/nikandfor/tlog/low"
)

type (
	Decoder struct {
		io.Reader

		err error

		b   []byte
		ref int
	}

	Dumper struct {
		io.Writer

		d Decoder

		NoGlobalOffset bool

		ref int
		b   low.Buf
	}
)

func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{
		Reader: r,
	}
}

func NewDecoderBytes(b []byte) *Decoder {
	return &Decoder{
		b: b,
	}
}

func (d *Decoder) Reset(r io.Reader) {
	d.Reader = r
	d.b = d.b[:0]
	d.ref = 0
	d.err = nil
}

func (d *Decoder) ResetBytes(b []byte) {
	d.b = b
	d.ref = 0
	d.err = nil
}

func (d *Decoder) Skip(st int) (i int) {
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

func (d *Decoder) ID(st int) (id ID, i int) {
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

func (d *Decoder) Location(st int) (pc loc.PC, i int) {
	tag, sub, i := d.Tag(st)
	if d.err != nil {
		return
	}

	if tag != Semantic || sub != WireLocation {
		d.newErr(st, "expected location")
		return
	}

	st = i
	tag, sub, i = d.Tag(i)
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

func (d *Decoder) Labels(st int) (ls Labels, i int) {
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

	for el := 0; sub == -1 || el < sub; el++ {
		if sub == -1 && d.Break(&i) {
			break
		}

		s, i := d.String(i)
		if d.err != nil {
			return nil, i
		}

		ls = append(ls, string(s))
	}

	return
}

func (d *Decoder) Time(st int) (ts Timestamp, i int) {
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

func (d *Decoder) LogLevel(st int) (lv LogLevel, i int) {
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

func (d *Decoder) String(st int) (s []byte, i int) {
	tag, l, i := d.Tag(st)

	if tag != String && tag != Bytes {
		d.newErr(st, "wanted string/bytes")
		return
	}

	if !d.more(st, i+l) {
		return
	}

	s = d.b[i : i+l]
	i += l

	return
}

func (d *Decoder) Float(st int) (v float64, i int) {
	tag, sub, i := d.Tag(st)

	if tag != Special {
		d.newErr(st, "wanted float")
		return
	}

	switch sub {
	case FloatInt8:
		if !d.more(st, 1) {
			return
		}

		q := int8(d.b[i])
		i++

		return float64(q), i
	case Float32:
		if !d.more(st, 4) {
			return
		}

		q := uint32(d.b[i])<<24 | uint32(d.b[i+1])<<16 | uint32(d.b[i+2])<<8 | uint32(d.b[i+3])
		i += 4

		return float64(math.Float32frombits(q)), i
	case Float64:
		if !d.more(st, 8) {
			return
		}

		q := uint64(d.b[i])<<56 | uint64(d.b[i+1])<<48 | uint64(d.b[i+2])<<40 | uint64(d.b[i+3])<<32 |
			uint64(d.b[i+4])<<24 | uint64(d.b[i+5])<<16 | uint64(d.b[i+6])<<8 | uint64(d.b[i+7])
		i += 8

		return math.Float64frombits(q), i
	case -1:
	default:
		d.newErr(st, "unsupported float specials: %x", sub)
	}

	return
}

func (d *Decoder) Int(st int) (v int64, i int) {
	if !d.more(st, st+1) {
		return -1, st
	}

	i = st
	tag := int(d.b[i] & TypeMask)
	if tag != Int && tag != Neg {
		d.newErr(st, "expected int: got %x", tag)
		return
	}

	v = int64(d.b[i] & TypeDetMask)
	i++

	switch v {
	case LenBreak:
		d.newErr(st, "unsupported break in int")
	case Len1:
		if !d.more(st, i+1) {
			return -1, i
		}

		v = int64(d.b[i])
		i++
	case Len2:
		if !d.more(st, i+2) {
			return -1, i
		}

		v = int64(d.b[i])<<8 | int64(d.b[i+1])
		i += 2
	case Len4:
		if !d.more(st, i+4) {
			return -1, i
		}

		v = int64(d.b[i])<<24 | int64(d.b[i+1])<<16 | int64(d.b[i+2])<<8 | int64(d.b[i+3])
		i += 4
	case Len8:
		if !d.more(st, i+8) {
			return -1, i
		}

		v = int64(d.b[i])<<56 | int64(d.b[i+1])<<48 | int64(d.b[i+2])<<40 | int64(d.b[i+3])<<32 |
			int64(d.b[i+4])<<24 | int64(d.b[i+5])<<16 | int64(d.b[i+6])<<8 | int64(d.b[i+7])
		i += 8
	}

	if tag == Neg {
		v = -v
	}

	return
}

func (d *Decoder) Tag(st int) (tag, sub, i int) {
	if !d.more(st, st+1) {
		return -1, -1, st
	}

	i = st
	tag = int(d.b[i] & TypeMask)
	sub = int(d.b[i] & TypeDetMask)
	i++

	if tag == Special {
		return
	}

	switch sub {
	case LenBreak:
		sub = -1
	case Len1:
		if !d.more(st, i+1) {
			return -1, -1, i
		}

		sub = int(d.b[i])
		i++
	case Len2:
		if !d.more(st, i+2) {
			return -1, -1, i
		}

		sub = int(d.b[i])<<8 | int(d.b[i+1])
		i += 2
	case Len4:
		if !d.more(st, i+4) {
			return -1, -1, i
		}

		sub = int(d.b[i])<<24 | int(d.b[i+1])<<16 | int(d.b[i+2])<<8 | int(d.b[i+3])
		i += 4
	case Len8:
		if !d.more(st, i+8) {
			return -1, -1, i
		}

		sub = int(d.b[i])<<56 | int(d.b[i+1])<<48 | int(d.b[i+2])<<40 | int(d.b[i+3])<<32 | int(d.b[i+4])<<24 | int(d.b[i+5])<<16 | int(d.b[i+6])<<8 | int(d.b[i+7])
		i += 8
	}

	return
}

func (d *Decoder) Break(i *int) bool {
	if !d.more(*i, *i+1) {
		return true
	}

	if d.b[*i] != Special|Break {
		return false
	}

	(*i)++

	return true
}

func (d *Decoder) more(st, end int) bool {
	if d.err != nil {
		return false
	}

	if end <= len(d.b) {
		return true
	}

	if d.Reader == nil {
		d.newErr(st, "short buffer, no reader")
		return false
	}

	// [0] already-used [st] not-used [len(d.b)] free-space [cap(d.b)]
	// d.ref = start of d.b position in stream

	read := len(d.b)

	if cap(d.b) < end {
		c := cap(d.b) * 5 / 4
		if c < end {
			c = end
		}
		b := make([]byte, c)
		copy(b, d.b)
		d.b = b
	}

	d.b = d.b[:cap(d.b)]

more:
	n, err := d.Reader.Read(d.b[read:end])
	read += n

	if err != nil {
		d.err = err
		return false
	}

	if end <= read {
		d.b = d.b[:read]
		return true
	}

	goto more
}

func (d *Decoder) newErr(i int, f string, args ...interface{}) {
	if d.err != nil {
		return
	}

	d.err = fmt.Errorf("%w (pos %x (%x) of %x) (%v)", fmt.Errorf(f, args...), i, (d.ref + i), len(d.b), loc.Callers(2, 3))
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

	i := 0
	for i < len(p) {
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

func (w *Dumper) dump(st, d int) (i int) {
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
			st := i
			if sub == -1 && w.d.Break(&i) {
				i = w.dump(st, d+1)
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
	d.Write(p)

	return string(d.b)
}
