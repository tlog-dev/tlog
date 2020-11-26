package wire

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"runtime/debug"
	"time"

	"github.com/nikandfor/tlog/core"
	"github.com/nikandfor/tlog/loc"
	"github.com/nikandfor/tlog/low"
)

type (
	Decoder struct {
		Labels core.Labels
	}

	EventHeader struct {
		Time    int64
		Type    core.Type
		Level   core.Level
		PC      loc.PC
		Labels  core.Labels
		Span    core.ID
		Parent  core.ID
		Elapsed time.Duration
		Message []byte
		Value   interface{}

		MoreTags []Tag
	}

	Dumper struct {
		io.Writer

		l int
		d Decoder

		NoGlobalOffset bool
	}

	perr error

	undef struct{}
)

var UndefVal undef

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

func (d *Decoder) SkipRecord(b []byte, i int) (_ int, err error) {
	//	fmt.Fprintf(os.Stderr, "message\n%v", Dump(b))

	defer func() {
		p := recover()
		if p == nil {
			return
		}

		if p, ok := p.(perr); ok {
			err = p
			return
		}

		panic(p)
	}()

	els, i := d.NextInt(b, i)

	for el := 0; i < len(b) && els == -1 || el < els; el++ {
		//	fmt.Fprintf(os.Stderr, "parsetag  i %3x  t %2x\n", i, b[i])

		if els == -1 && b[i] == Spec|Break {
			break
		}

		i = d.Skip(b, i)
	}

	return i, nil
}

func (d *Decoder) Skip(b []byte, i int) int {
	return dump(ioutil.Discard, -1, b, i, 0)
}

func (d *Decoder) DecodeHeader(b []byte, ev *EventHeader) (i int, err error) {
	//	fmt.Fprintf(os.Stderr, "message\n%v", Dump(b))

	defer func() {
		p := recover()
		if p == nil {
			return
		}

		if p, ok := p.(perr); ok {
			err = p
			return
		}

		panic(p)
	}()

	els, i := d.NextInt(b, i)

	var v interface{}
	var q int64
	var t byte
	var s []byte
loop:
	for el := 0; i < len(b) && els == -1 || el < els; el++ {
		t = b[i]
		i++

		//	fmt.Fprintf(os.Stderr, "parsetag  i %3x  t %2x\n", i-1, t)

		if els == -1 && t == Spec|Break {
			break
		}

		if t&TypeMask != Semantic {
			return i, fmt.Errorf("unexpected tag, expected semantic: %2x (pos %x)", t, i-1)
		}

		t &^= TypeMask

		switch t {
		case UserFields:
			i--

			break loop
		case Time:
			if b[i] != Semantic|Time {
				return i, fmt.Errorf("expected timestamp, got: %2x", b[i])
			}
			i++

			ev.Time, i = d.NextInt64(b, i)
		case Type:
			switch b[i] & TypeMask {
			case Int:
				q, i = d.NextInt64(b, i)
				ev.Type = core.Type(q)
			case String:
				s, i = d.NextString(b, i)
				ev.Type = core.Type(s[0])
			default:
				return i, fmt.Errorf("parse Type: expected Int or String, got %2x (pos %x)", b[i], i)
			}
		case Level:
			q, i = d.NextInt64(b, i)

			ev.Level = core.Level(q)
		case Location:
			ev.PC, i = d.NextLoc(b, i)
		case Labels:
			ev.Labels, i = d.NextLabels(b, i)
		case Span:
			ev.Span, i = d.NextID(b, i)
		case Parent:
			ev.Parent, i = d.NextID(b, i)
		case SpanElapsed:
			ev.Elapsed, i = d.NextDuration(b, i)
		case Message:
			ev.Message, i = d.NextString(b, i)
		case Value:
			ev.Value, i = d.NextValue(b, i)
		default:
			v, i = d.NextValue(b, i)

			ev.MoreTags = append(ev.MoreTags, Tag{T: Semantic | int(t), V: v})
		}
	}

	return i, nil
}

func (d *Decoder) Tag(b []byte, i int) byte {
	if i >= len(b) {
		return 0xff
	}

	return b[i]
}

func (d *Decoder) NextValue(b []byte, i int) (interface{}, int) {
	var q int64
	var s []byte

	switch b[i] & TypeMask {
	case Int:
		q, i = d.NextInt64(b, i)
		if q < 0 {
			return uint64(q), i
		}

		return q, i
	case Neg:
		return d.NextInt64(b, i)
	case Bytes:
		return d.NextString(b, i)
	case String:
		s, i = d.NextString(b, i)

		return string(s), i
	case Array:
		var els int
		els, i = d.NextInt(b, i)

		var v []interface{}
		if els != -1 {
			v = make([]interface{}, 0, els)
		}

		var vi interface{}
		for el := 0; els == -1 || el < els; el++ {
			if els == -1 && b[i] == Spec|Break {
				break
			}

			vi, i = d.NextValue(b, i)

			v = append(v, vi)
		}

		return v, i
	case Map:
		var els int
		els, i = d.NextInt(b, i)

		var v map[string]interface{}
		if els != -1 {
			v = make(map[string]interface{}, els)
		}

		var ki, vi interface{}
		for el := 0; els == -1 || el < els; el++ {
			if els == -1 && b[i] == Spec|Break {
				break
			}

			ki, i = d.NextValue(b, i)
			vi, i = d.NextValue(b, i)

			v[ki.(string)] = vi
		}

		return v, i
	case Semantic:
		switch b[i] & TypeDetMask {
		case Time:
			q, i = d.NextInt64(b, i)

			return time.Unix(0, q), i
		case Duration:
			return d.NextDuration(b, i)
		case ID:
			return d.NextID(b, i)
		case Location:
			return d.NextLoc(b, i)
		case Labels:
			return d.NextLabels(b, i)
		case Error:
			var sub interface{}
			sub, i = d.NextValue(b, i+1)

			switch v := sub.(type) {
			case []byte:
				return errors.New(low.UnsafeBytesToString(v)), i
			default:
				return v, i
			}
		default:
			return d.NextValue(b, i+1)
		}
	case Spec:
		i++
		switch b[i-1] & TypeDetMask {
		case False:
			return false, i
		case True:
			return true, i
		case Null:
			return nil, i
		case Undefined:
			return UndefVal, i
		case Float32:
			f := math.Float32frombits(binary.BigEndian.Uint32(b[i:]))
			return float64(f), i + 4
		case Float64:
			f := math.Float64frombits(binary.BigEndian.Uint64(b[i:]))
			return f, i + 8
		default:
			panic(perr(fmt.Errorf("unsupported special value: %2x", b[i-1])))
		}

	default:
		panic(perr(fmt.Errorf("unsupported type: %2x", b[i])))
	}
}

func (d *Decoder) NextLabels(b []byte, i int) (ls core.Labels, _ int) {
	i++ // semantic header

	if b[i]&TypeMask != Array {
		panic(perr(fmt.Errorf("parsing id: expected bytes, got %2x", b[i])))
	}

	l, i := d.NextInt(b, i)

	var q []byte
	for j := 0; j < l; j++ {
		q, i = d.NextString(b, i)

		ls = append(ls, string(q))
	}

	return ls, i
}

func (d *Decoder) NextID(b []byte, i int) (id core.ID, _ int) {
	i++ // semantic header

	if b[i]&TypeMask != Bytes {
		panic(perr(fmt.Errorf("parsing id: expected bytes, got %2x", b[i])))
	}

	tl := b[i] & TypeDetMask
	i++

	copy(id[:], b[i:])

	return id, i + int(tl)
}

func (d *Decoder) NextLoc(b []byte, i int) (loc.PC, int) {
	i++ // semantic header

	t := b[i] & TypeMask

	var pc int64

	if t == Int {
		pc, i = d.NextInt64(b, i)

		return loc.PC(pc), i
	}

	if t != Map {
		panic(perr(fmt.Errorf("parsing location: expected map, got %2x (pos %x)", t, i)))
	}

	tl := int(b[i] & TypeDetMask)
	i++

	var name, file string
	var line int

	var k []byte
	for j := 0; j < tl; j++ {
		k, i = d.NextString(b, i)

		switch k[0] {
		case 'p':
			pc, i = d.NextInt64(b, i)
		case 'n':
			k, i = d.NextString(b, i)
			name = string(k)
		case 'f':
			k, i = d.NextString(b, i)
			file = string(k)
		case 'l':
			line, i = d.NextInt(b, i)
		default:
			panic(perr(fmt.Errorf("parsing location: unexpected key %q", k)))
		}
	}

	q := loc.PC(pc)
	q.SetCache(name, file, line)

	return q, i
}

func (d *Decoder) NextString(b []byte, i int) (v []byte, _ int) {
	t := b[i] & TypeMask
	if t != String && t != Bytes {
		panic(perr(fmt.Errorf("expected string or bytes, got %2x (pos %x)", t, i)))
	}

	l, i := d.NextInt(b, i)

	return b[i : i+l], i + l
}

func (d *Decoder) NextDuration(b []byte, i int) (time.Duration, int) {
	i++ // semantic header

	q, i := d.NextInt64(b, i)
	return time.Duration(q), i
}

func (d *Decoder) NextInt(b []byte, i int) (v int, _ int) {
	q, i := d.NextInt64(b, i)
	return int(q), i
}

func (d *Decoder) NextInt64(b []byte, i int) (v int64, _ int) {
	tl := b[i] & TypeDetMask
	i++

	switch tl {
	default:
		v = int64(tl)
	case LenBreak:
		v = -1
	case Len8:
		v |= int64(b[i]) << 56
		i++
		v |= int64(b[i]) << 48
		i++
		v |= int64(b[i]) << 40
		i++
		v |= int64(b[i]) << 32
		i++

		fallthrough
	case Len4:
		v |= int64(b[i]) << 24
		i++
		v |= int64(b[i]) << 16
		i++

		fallthrough
	case Len2:
		v |= int64(b[i]) << 8
		i++

		fallthrough
	case Len1:
		v |= int64(b[i])
		i++
	}

	return v, i
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
			if l == -1 && b[i] == Spec|Break {
				i = dump(w, base, b, i, d+1)
				break
			}

			i = dump(w, base, b, i, d+1)
		}
	case Map:
		fmt.Fprintf(w, "object: len %v\n", l)

		for j := 0; l == -1 || j < int(l); j++ {
			if l == -1 && b[i] == Spec|Break {
				i = dump(w, base, b, i, d+1)
				break
			}

			i = dump(w, base, b, i, d+1)
			i = dump(w, base, b, i, d+1)
		}
	case Semantic:
		fmt.Fprintf(w, "semantic %2x\n", l)

		i = dump(w, base, b, i, d+1)
	case Spec:
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

	if t == Spec {
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
