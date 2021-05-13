package wire

import (
	"encoding/hex"
	"fmt"
	"io"

	"github.com/nikandfor/tlog/low"
)

type (
	Dumper struct {
		LowDecoder

		io.Writer
		pos int64

		NoGlobalOffset bool

		b low.Buf
	}
)

func Dump(p []byte) (r string) {
	var d Dumper
	d.NoGlobalOffset = true

	defer func() {
		e := recover()
		if e == nil {
			return
		}

		r = fmt.Sprintf("%s\npanic: %v\nhex.Dump\n%s", d.b, e, hex.Dump(p))
	}()

	_, _ = d.Write(p)

	d.b.NewLine()

	return string(d.b)
}

func NewDumper(w io.Writer) *Dumper {
	return &Dumper{Writer: w}
}

func (d *Dumper) Write(p []byte) (n int, err error) {
	d.b = d.b[:0]

	var i int

	for i < len(p) {
		i = d.dump(p, i, 0)
	}

	d.pos += int64(i)

	if d.Writer != nil {
		n, err = d.Writer.Write(d.b)
	}

	return
}

func (d *Dumper) dump(p []byte, st, depth int) (i int) {
	tag, sub, i := d.Tag(p, st)

	if !d.NoGlobalOffset {
		d.b = low.AppendPrintf(d.b, "%8x  ", d.pos+int64(st))
	}

	d.b = low.AppendPrintf(d.b, "%4x  %s% x  -  ", st, low.Spaces[:depth*2], p[st:i])

	switch tag {
	case Int, Neg:
		var v int64
		v, i = d.Signed(p, st)

		d.b = low.AppendPrintf(d.b, "int %10v\n", v)
	case Bytes, String:
		var s []byte
		s, i = d.String(p, st)

		if tag == Bytes {
			d.b = low.AppendPrintf(d.b, "% x\n", s)
		} else {
			d.b = low.AppendPrintf(d.b, "%q\n", s)
		}
	case Array, Map:
		tg := "array"
		if tag == Map {
			tg = "map"
		}

		d.b = low.AppendPrintf(d.b, "%v: len %v\n", tg, sub)

		for el := 0; sub == -1 || el < int(sub); el++ {
			if sub == -1 && d.Break(p, &i) {
				i = d.dump(p, i-1, depth+1)
				break
			}

			i = d.dump(p, i, depth+1)

			if tag == Map {
				i = d.dump(p, i, depth+1)
			}
		}
	case Semantic:
		d.b = low.AppendPrintf(d.b, "semantic %2x\n", sub)

		i = d.dump(p, i, depth+1)
	case Special:
		switch sub {
		case False:
			d.b = low.AppendPrintf(d.b, "false\n")
		case True:
			d.b = low.AppendPrintf(d.b, "true\n")
		case Null:
			d.b = low.AppendPrintf(d.b, "null\n")
		case Undefined:
			d.b = low.AppendPrintf(d.b, "undefined\n")
		case Float8, Float16, Float32, Float64:
			var f float64

			f, i = d.Float(p, st)

			d.b = low.AppendPrintf(d.b, "%v\n", f)
		case Break:
			d.b = low.AppendPrintf(d.b, "break\n")
		default:
			d.b = low.AppendPrintf(d.b, "special: %x\n", sub)

			panic("unsupported special")
		}
	}

	return
}
