package tlog

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"runtime/debug"

	"github.com/nikandfor/tlog/low"
)

type (
	Decoder struct {
	}

	Dumper struct {
		io.Writer

		l int
		d Decoder

		NoGlobalOffset bool
	}
)

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
