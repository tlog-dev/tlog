package compress

import (
	"fmt"
	"io"
	"os"
	"unsafe"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/low"
)

type (
	Encoder struct {
		io.Writer

		b       []byte
		written int64

		block []byte
		pos   int64
		mask  int64

		ht    []uint32
		hmask uintptr
	}
)

// tags
const (
	Literal = iota << 6
	Copy
	Entropy
	Meta

	TagMask    = 0b1100_0000
	TagLenMask = 0b0011_1111
)

// tag lengths
const (
	_ = 1<<6 - iota
	TagLen8
	TagLen4
	TagLen2
	TagLen1
)

// offset lengths
const (
	_ = 1<<8 - iota
	Off8
	Off4
	Off2
	Off1
)

// meta tags
const (
	_ = iota
	MetaReset

//	MetaBlockSize
)

var zeros = make([]byte, 1024)

var tl *tlog.Logger

func NewEncoder(w io.Writer, bs int) *Encoder {
	if bs&(bs-1) != 0 || bs < 1<<16 {
		panic(bs)
	}

	return newEncoder(w, bs, 6)
}

func newEncoder(w io.Writer, bs, ss int) *Encoder {
	hlen := bs >> ss

	return &Encoder{
		Writer: w,
		block:  make([]byte, bs),
		mask:   int64(bs - 1),
		ht:     make([]uint32, hlen),
		hmask:  uintptr(hlen - 1),
	}
}

func (w *Encoder) Reset(wr io.Writer) {
	w.Writer = wr
	w.pos = 0
	for i := 0; i < len(w.block); {
		i += copy(w.block[i:], zeros)
	}
	for i := range w.ht {
		w.ht[i] = 0
	}
}

func (w *Encoder) Write(d []byte) (_ int, err error) {
	if w.pos == 0 {
		w.b = w.appendHeader(w.b)
	}

	msgst := w.pos
	var done, i int64 = 0, 0

	for i+4 <= int64(len(d)) {
		h := low.MemHash32(unsafe.Pointer(&d[i]), 0)
		h &= w.hmask

		p := int64(w.ht[h])
		w.ht[h] = uint32(msgst + i)

		wposwas := w.pos
		p1 := p
		g := i - done

		danger := i - done

		if uint32(w.pos-p) > uint32(len(w.block)) || p+danger > w.pos || p+int64(len(w.block)) < w.pos+danger { // range is more than block size or copy in a danger zone (near w.pos&w.mask)
			//		tl.Printw("skip hash", "p", tlog.Hex(p), "w.pos", tlog.Hex(w.pos), "diff", tlog.Hex(w.pos-p), "block", tlog.Hex(len(w.block)))
			i++
			continue
		}

		p = w.pos - int64(uint32(w.pos-p))

		//	tl.Printw("hash", "p", tlog.Hex(p), "i_", tlog.Hex(i), "len", tlog.Hex(len(d)), "newp", tlog.Hex((w.pos+i)&w.mask), "h", tlog.Hex(h),
		//		"data", tlog.FormatNext("%.8s"), d[i:],
		//		"block", tlog.FormatNext("%.8s"), w.block[p&w.mask:])

		st, end := w.compare(d[done:], i-done, p)
		if end > w.pos {
			end = w.pos
		}
		//	tl.Printw("compare", "p", tlog.Hex(p), "st", tlog.Hex(st), "end", tlog.Hex(end), "size", tlog.Hex(end-st), "w.pos", tlog.Hex(w.pos))
		if end-st <= 6 {
			i++
			continue
		}

		i -= p - st

		if done < i {
			w.appendLiteral(d, done, i)
		}

		if w.pos-st > int64(len(w.block)) {
			fmt.Fprintf(os.Stderr, "offset is out of range  wpos %x (%x)  st %x  end %x  block %x  p %x (%x)  offst %x  offend %x  size %x  g %x\n", w.pos, wposwas, st, end, len(w.block), p, p1, w.pos-st, w.pos-end, end-st, g)
			//	panic("offset is out of range")
		}

		w.appendCopy(st, end)
		i += end - st

		done = i
	}

	if done < int64(len(d)) {
		w.appendLiteral(d, done, int64(len(d)))
		done = int64(len(d))
	}

	n, err := w.Writer.Write(w.b)
	w.written += int64(n)

	w.b = w.b[:0]

	//	tl.Printf("ht\n%x", w.ht)
	//	tl.Printf("block\n%v", hex.Dump(w.block))

	return int(done), err
}

func (w *Encoder) appendHeader(b []byte) []byte {
	bs := 0
	for q := len(w.block); q != 1; q >>= 1 {
		bs++
	}

	//	tl.Printw("meta", "sub", tlog.Hex(MetaReset), "sub_name", "reset", "block_size", bs)

	b = append(b, Meta|MetaReset, byte(bs))

	return b
}

func (w *Encoder) appendLiteral(d []byte, s, e int64) {
	//	tl.Printw("literal", "st", tlog.Hex(s), "end", tlog.Hex(e), "size", tlog.Hex(e-s), "w.pos", tlog.Hex(w.pos), "caller", loc.Caller(1))

	w.b = w.appendTag(w.b, Literal, e-s)
	w.b = append(w.b, d[s:e]...)

	for s < e {
		n := int64(copy(w.block[w.pos&w.mask:], d[s:e]))
		s += n
		w.pos += n
	}
}

func (w *Encoder) appendCopy(st, end int64) {
	w.b = w.appendTag(w.b, Copy, end-st)
	w.b = w.appendOff(w.b, w.pos-end)

	//	tl.Printw("copy", "st", tlog.Hex(st), "end", tlog.Hex(end), "size", tlog.Hex(end-st), "w.pos", tlog.Hex(w.pos), "off", tlog.Hex(w.pos-st))

	var n int
	for st < end {
		if st&w.mask < end&w.mask {
			n = copy(w.block[w.pos&w.mask:], w.block[st&w.mask:end&w.mask])
		} else {
			n = copy(w.block[w.pos&w.mask:], w.block[st&w.mask:])
		}
		w.pos += int64(n)
		st += int64(n)
	}
}

func (w *Encoder) compare(d []byte, i, p int64) (st, end int64) {
	// move end
	end = p & w.mask
	base := p - end

moreend:
	for i+8 <= int64(len(d)) && end+8 <= int64(len(w.block)) {
		if *(*uint64)(unsafe.Pointer(&d[i])) != *(*uint64)(unsafe.Pointer(&w.block[end])) {
			break
		}

		end += 8
		i += 8
	}

	for i < int64(len(d)) && end < int64(len(w.block)) {
		if d[i] != w.block[end] {
			break
		}

		end++
		i++
	}

	if end == int64(len(w.block)) && i != int64(len(d)) {
		base += int64(len(w.block))
		end = 0

		goto moreend
	}

	end += base

	// move st
	i -= end - p

	st = p & w.mask
	base = p - st

morest:
	for i-8 >= 0 && st-8 >= 0 {
		if *(*uint64)(unsafe.Pointer(&d[i-8])) != *(*uint64)(unsafe.Pointer(&w.block[st-8])) {
			break
		}

		st -= 8
		i -= 8
	}

	for i > 0 && st > 0 {
		if d[i-1] != w.block[st-1] {
			break
		}

		st--
		i--
	}

	if st == 0 && i != 0 {
		base -= int64(len(w.block))
		st = int64(len(w.block))

		goto morest
	}

	st += base

	return st, end
}

func (w *Encoder) appendTag(b []byte, tag byte, l int64) []byte {
	switch {
	case l < TagLen1:
		return append(b, tag|byte(l))
	case l <= 0xff:
		return append(b, tag|TagLen1, byte(l))
	case l <= 0xffff:
		return append(b, tag|TagLen2, byte(l>>8), byte(l))
	case l <= 0xffff_ffff:
		return append(b, tag|TagLen4, byte(l>>24), byte(l>>16), byte(l>>8), byte(l))
	default:
		return append(b, tag|TagLen8, byte(l>>56), byte(l>>48), byte(l>>40), byte(l>>32), byte(l>>24), byte(l>>16), byte(l>>8), byte(l))
	}
}

func (w *Encoder) appendOff(b []byte, l int64) []byte {
	switch {
	case l < Off1:
		return append(b, byte(l))
	case l <= 0xff:
		return append(b, Off1, byte(l))
	case l <= 0xffff:
		return append(b, Off2, byte(l>>8), byte(l))
	case l <= 0xffff_ffff:
		return append(b, Off4, byte(l>>24), byte(l>>16), byte(l>>8), byte(l))
	default:
		return append(b, Off8, byte(l>>56), byte(l>>48), byte(l>>40), byte(l>>32), byte(l>>24), byte(l>>16), byte(l>>8), byte(l))
	}
}
