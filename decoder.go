package tlog

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"runtime/debug"

	"github.com/nikandfor/loc"
	"github.com/nikandfor/tlog/low"
)

type (
	Decoder struct {
		Err error
	}

	Dumper struct {
		io.Writer

		l int
		d Decoder

		NoGlobalOffset bool
	}
)

func NewDecoder() *Decoder {
	return &Decoder{}
}

func (d *Decoder) SkipNext(p []byte, st int) int {
	if d.Err != nil {
		return st
	}

	return dump(nil, 0, p, st, 0)
}

func (d *Decoder) NextBreak(p []byte, i *int) (ok bool) {
	if d.Err != nil {
		return false
	}

	if *i >= len(p) {
		d.Err = errors.New("expected break")
		return false
	}

	if p[*i] != Special|Break {
		return false
	}

	(*i)++

	return true
}

func (d *Decoder) NextID(p []byte, st int) (id ID, i int) {
	if d.Err != nil {
		return ID{}, st
	}

	i = st
	if i+3 >= len(p) {
		d.Err = errors.New("expected id")
		return ID{}, st
	}

	if p[i] != Semantic|WireID {
		d.Err = errors.New("expected id")
		return ID{}, st
	}
	i++

	_, l, i := d.NextString(p, i)

	copy(id[:], l)

	return id, i
}

func (d *Decoder) NextLoc(p []byte, st int) (lc loc.PC, i int) {
	if d.Err != nil {
		return 0, st
	}

	i = st
	if i+2 >= len(p) {
		d.Err = errors.New("expected location")
		return 0, st
	}

	if p[i] != Semantic|WireLocation {
		d.Err = errors.New("expected location")
		return 0, st
	}
	i++

	tag := p[i] & TypeMask
	if tag == Int {
		var v int64
		_, v, i = d.NextInt(p, i)

		return loc.PC(v), i
	}

	if tag != Map {
		d.Err = errors.New("expected location (map)")
		return 0, st
	}

	_, els, i := d.NextTag(p, i)

	var name, file string
	var line int

	var k []byte
	var v int64
	for el := 0; els == -1 || el < els; el++ {
		if els == -1 && d.NextBreak(p, &i) {
			break
		}

		_, k, i = d.NextString(p, i)
		if len(k) == 0 {
			d.Err = errors.New("empty key")
			return 0, st
		}

		switch k[0] {
		case 'p':
			_, v, i = d.NextInt(p, i)
			lc = loc.PC(v)
		case 'n':
			_, k, i = d.NextString(p, i)
			name = string(k)
		case 'f':
			_, k, i = d.NextString(p, i)
			file = string(k)
		case 'l':
			_, line, i = d.NextTag(p, i)
		default:
			i = d.SkipNext(p, i)
		}
	}

	if lc != 0 {
		lc.SetCache(name, file, line)
	}

	return lc, i
}

func (d *Decoder) NextLabels(p []byte, st int) (ls Labels, i int) {
	if d.Err != nil {
		return nil, st
	}

	i = st
	if i+2 >= len(p) {
		d.Err = errors.New("expected labels")
		return nil, st
	}

	if p[i] != Semantic|WireLabels {
		d.Err = errors.New("expected labels")
		return nil, st
	}
	i++

	tag, els, i := d.NextTag(p, i)
	if tag != Array {
		d.Err = errors.New("expected labels (array)")
		return nil, st
	}

	var s []byte
	for el := 0; els == -1 || el < els; el++ {
		if els == -1 {
			panic("implement")
		}

		_, s, i = d.NextString(p, i)

		ls = append(ls, string(s))
	}

	return ls, i
}

func (d *Decoder) NextTime(p []byte, st int) (ts Timestamp, i int) {
	if d.Err != nil {
		return 0, st
	}

	i = st
	if i+2 >= len(p) {
		d.Err = io.ErrUnexpectedEOF
		return 0, st
	}

	if p[i] != Semantic|WireTime {
		d.Err = errors.New("expected time")
		return 0, st
	}
	i++

	tag, v, i := d.NextInt(p, i)
	if tag != Int {
		d.Err = errors.New("expected time (int)")
		return 0, st
	}

	return Timestamp(v), i
}

func (d *Decoder) NextString(p []byte, st int) (tag int, s []byte, i int) {
	tag, l, i := d.NextTag(p, st)
	if d.Err != nil {
		return tag, nil, st
	}

	if tag != String && tag != Bytes {
		d.Err = errors.New("expected string or bytes")
		return tag, nil, st
	}

	if i+l > len(p) {
		d.Err = io.ErrUnexpectedEOF
		return tag, nil, st
	}

	return tag, p[i : i+l], i + l
}

func (d *Decoder) NextInt(p []byte, st int) (tag int, l int64, i int) {
	if d.Err != nil {
		return tag, -1, i
	}

	i = st
	if i == len(p) {
		d.Err = io.ErrUnexpectedEOF
		return tag, -1, i
	}

	tag = int(p[i] & TypeMask)
	l = int64(p[i] & TypeDetMask)
	i++

	switch l {
	case LenBreak:
		d.Err = errors.New("unexpected break len")

		return tag, -1, st
	case Len1:
		if i+1 > len(p) {
			d.Err = io.ErrUnexpectedEOF
			return tag, -1, i
		}

		l = int64(p[i])
		i++
	case Len2:
		if i+2 > len(p) {
			d.Err = io.ErrUnexpectedEOF
			return tag, -1, i
		}

		l = int64(p[i])<<8 | int64(p[i+1])
		i += 2
	case Len4:
		if i+4 > len(p) {
			d.Err = io.ErrUnexpectedEOF
			return tag, -1, i
		}

		l = int64(p[i])<<24 | int64(p[i+1])<<16 | int64(p[i+2])<<8 | int64(p[i+3])
		i += 4
	case Len8:
		if i+8 > len(p) {
			d.Err = io.ErrUnexpectedEOF
			return tag, -1, i
		}

		l = int64(p[i])<<56 | int64(p[i+1])<<48 | int64(p[i+2])<<40 | int64(p[i+3])<<32 | int64(p[i+4])<<24 | int64(p[i+5])<<16 | int64(p[i+6])<<8 | int64(p[i+7])
		i += 8
	}

	if tag == Neg {
		l = -l
	}

	return tag, l, i
}

func (d *Decoder) NextTag(p []byte, st int) (tag, l, i int) {
	if d.Err != nil {
		return tag, -1, st
	}

	i = st
	if i == len(p) {
		d.Err = io.ErrUnexpectedEOF
		return tag, -1, st
	}

	tag = int(p[i] & TypeMask)
	l = int(p[i] & TypeDetMask)
	i++

	if tag == Special {
		return tag, l, i
	}

	switch l {
	case LenBreak:
		l = -1
	case Len1:
		if i+1 > len(p) {
			d.Err = io.ErrUnexpectedEOF
			return tag, -1, i
		}

		l = int(p[i])
		i++
	case Len2:
		if i+2 > len(p) {
			d.Err = io.ErrUnexpectedEOF
			return tag, -1, i
		}

		l = int(p[i])<<8 | int(p[i+1])
		i += 2
	case Len4:
		if i+4 > len(p) {
			d.Err = io.ErrUnexpectedEOF
			return tag, -1, i
		}

		l = int(p[i])<<24 | int(p[i+1])<<16 | int(p[i+2])<<8 | int(p[i+3])
		i += 4
	case Len8:
		if i+8 > len(p) {
			d.Err = io.ErrUnexpectedEOF
			return tag, -1, i
		}

		l = int(p[i])<<56 | int(p[i+1])<<48 | int(p[i+2])<<40 | int(p[i+3])<<32 | int(p[i+4])<<24 | int(p[i+5])<<16 | int(p[i+6])<<8 | int(p[i+7])
		i += 8
	}

	return tag, l, i
}

func (d *Decoder) NextFloat(p []byte, st int) (v float64, i int) {
	tag, sub, i := d.NextTag(p, st)
	if d.Err != nil {
		return 0, st
	}

	switch {
	case tag == Int || tag == Neg:
		var v int64
		_, v, i = d.NextInt(p, st)

		return float64(v), i
	case tag != Special:
		d.Err = errors.New("expected float (tag)")

		return 0, st
	case sub == Float32:
		if i+4 > len(p) {
			d.Err = io.ErrUnexpectedEOF
			return 0, i
		}

		q := uint32(p[i])<<24 | uint32(p[i+1])<<16 | uint32(p[i+2])<<8 | uint32(p[i+3])
		i += 4

		v = float64(math.Float32frombits(q))

		return v, i
	case sub == Float64:
		if i+8 > len(p) {
			d.Err = io.ErrUnexpectedEOF
			return 0, i
		}

		q := uint64(p[i])<<56 | uint64(p[i+1])<<48 | uint64(p[i+2])<<40 | uint64(p[i+3])<<32 | uint64(p[i+4])<<24 | uint64(p[i+5])<<16 | uint64(p[i+6])<<8 | uint64(p[i+7])
		i += 8

		v = math.Float64frombits(q)

		return v, i
	default:
		d.Err = errors.New("expected float (tag)")

		return 0, st
	}
}

func NewDumper(w io.Writer) *Dumper {
	return &Dumper{
		Writer: w,
	}
}

func (w *Dumper) Write(p []byte) (i int, err error) {
	for i < len(p) {
		off := -1
		if !w.NoGlobalOffset {
			off = w.l + i
		}

		i = dump(w.Writer, off, p, i, 0)
	}

	if i != len(p) && err == nil {
		err = io.ErrUnexpectedEOF
	}

	w.l += i

	return i, nil
}

func Dump(b []byte) (r string) {
	var w low.Buf

	defer func() {
		perr := recover()
		if perr == nil {
			return
		}

		r = fmt.Sprintf("panic: %v\n", perr) + hex.Dump(b) + string(w) + "\n" + string(debug.Stack()) + "\n"
	}()

	for i := 0; i < len(b); {
		i = dump(&w, -1, b, i, 0)
	}

	return string(w)
}

func dump(w io.Writer, base int, b []byte, i, d int) int {
	if w == nil {
		w = ioutil.Discard
	}

	st := i
	t, l, i := decodetag(b, i)

	//	fmt.Fprintf(os.Stderr, "i %3x  t %2x l %x\n", i-1, t, l)

	if base != -1 {
		fmt.Fprintf(w, "%8x  ", base+st)
	}

	fmt.Fprintf(w, "%4x  %s% x  -  ", st, low.Spaces[:d*2], b[st:i])

	switch t {
	case Int:
		fmt.Fprintf(w, "int %10v\n", l)
	case Neg:
		fmt.Fprintf(w, "int %10v\n", -l)
	case Bytes:
		fmt.Fprintf(w, "% x\n", b[i:i+int(l)])
		i += int(l)
	case String:
		fmt.Fprintf(w, "%q\n", string(b[i:i+int(l)]))
		i += int(l)
	case Array:
		fmt.Fprintf(w, "array: len %v\n", l)

		for j := 0; l == -1 || j < int(l); j++ {
			if l == -1 && b[i] == Special|Break {
				i = dump(w, base, b, i, d+1)
				break
			}

			i = dump(w, base, b, i, d+1)
		}
	case Map:
		fmt.Fprintf(w, "object: len %v\n", l)

		for j := 0; l == -1 || j < int(l); j++ {
			if l == -1 && b[i] == Special|Break {
				i = dump(w, base, b, i, d+1)
				break
			}

			i = dump(w, base, b, i, d+1)
			i = dump(w, base, b, i, d+1)
		}
	case Semantic:
		fmt.Fprintf(w, "semantic %2x\n", l)

		i = dump(w, base, b, i, d+1)
	case Special:
		switch l {
		case False:
			fmt.Fprintf(w, "false")
		case True:
			fmt.Fprintf(w, "true")
		case Null:
			fmt.Fprintf(w, "null")
		case Undefined:
			fmt.Fprintf(w, "undefined")
		case Float64:
			v := math.Float64frombits(binary.BigEndian.Uint64(b[i:]))
			i += 8

			fmt.Fprintf(w, "%v", v)
		case Float32:
			v := math.Float32frombits(binary.BigEndian.Uint32(b[i:]))
			i += 4

			fmt.Fprintf(w, "%v", v)
		case 0x1f:
			fmt.Fprintf(w, "break")
		default:
			fmt.Fprintf(w, "special %x", l)
		}

		fmt.Fprintf(w, "\n")
	default:
		fmt.Fprintf(w, "unexpected type %2x\n", t)
	}

	return i
}

func decodetag(b []byte, i int) (t byte, l int64, _ int) {
	t = b[i] & TypeMask

	td := b[i] & TypeDetMask
	i++

	if t == Special {
		return t, int64(td), i
	}

	switch td {
	default:
		l = int64(td)
	case LenBreak:
		l = -1
	case Len8:
		l |= int64(b[i]) << 56
		i++
		l |= int64(b[i]) << 48
		i++
		l |= int64(b[i]) << 40
		i++
		l |= int64(b[i]) << 32
		i++

		fallthrough
	case Len4:
		l |= int64(b[i]) << 24
		i++
		l |= int64(b[i]) << 16
		i++

		fallthrough
	case Len2:
		l |= int64(b[i]) << 8
		i++

		fallthrough
	case Len1:
		l |= int64(b[i])
		i++
	}

	return t, l, i
}
