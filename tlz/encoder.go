package tlz

import (
	"io"
	"unsafe"
)

type (
	Encoder struct {
		io.Writer

		b       []byte
		written int64

		block []byte
		mask  int
		pos   int64

		ht    []uint32
		hmask uintptr
		hsh   uint
	}
)

// Byte multipliers.
const (
	B = 1 << (iota * 10)
	KiB
	MiB
	GiB
)

// Tags.
const (
	Literal = iota << 7
	Copy

	TagMask    = 0b1000_0000
	TagLenMask = 0b0111_1111
)

// Tag lengths.
const (
	_ = 1<<7 - iota
	Len8
	Len4
	Len2
	Len1

	Meta = 0 // Literal | Meta - means meta tag
)

// Offset lengths.
const (
	_ = 1<<8 - iota
	Off8
	Off4
	Off2
	Off1
)

// Meta tags.
const (
	MetaMagic    = iota << 4 // "tlz"
	MetaVer                  // Version
	MetaReset                // block size log
	MetaChecksum             // [...]byte

	MetaTagMask = 0xf0
)

const Version = "000" // must be less than 16 bytes

var zeros = make([]byte, 1024)

func NewEncoder(w io.Writer, bs int) *Encoder {
	if bs&(bs-1) != 0 || bs < 256 {
		panic("block size must be power of two and at least 1KB")
	}

	return NewEncoderHTSize(w, bs, bs>>6)
}

func newEncoder(w io.Writer, bs, ss int) *Encoder {
	return NewEncoderHTSize(w, bs, bs>>ss)
}

func NewEncoderHTSize(w io.Writer, bs, hlen int) *Encoder {
	if (bs-1)&bs != 0 {
		panic("bad block size")
	}

	hsh := uint(2)
	for 1<<(32-hsh) != hlen {
		hsh++
	}

	return &Encoder{
		Writer: w,
		block:  make([]byte, bs),
		mask:   bs - 1,
		ht:     make([]uint32, hlen),
		hmask:  uintptr(hlen - 1),
		hsh:    hsh,
	}
}

func (w *Encoder) Reset(wr io.Writer) {
	w.Writer = wr

	w.reset()
}

func (w *Encoder) reset() {
	w.pos = 0
	for i := 0; i < len(w.block); {
		i += copy(w.block[i:], zeros)
	}
	for i := range w.ht {
		w.ht[i] = 0
	}
}

// Write is io.Writer implementation.
func (w *Encoder) Write(p []byte) (done int, err error) { //nolint:gocognit
	w.b = w.b[:0]

	if w.pos == 0 {
		w.b = w.appendHeader(w.b)
	}

	start := int(w.pos)

	i := 0
	for i+4 < len(p) {
		h := *(*uint32)(unsafe.Pointer(&p[i])) * 0x1e35a7bd >> w.hsh

		pos := int(w.ht[h])
		w.ht[h] = uint32(start + i)

		if int(w.pos)-pos > len(w.block) || pos-int(w.pos) > 0 {
			i++
			continue
		}

		// grow forward
		iend := i
		end := pos
		for iend < len(p) && w.block[end&w.mask] == p[iend] {
			iend++
			end++
		}

		ist := i
		st := pos
		for ist > done && w.block[(st-1)&w.mask] == p[ist-1] {
			ist--
			st--
		}

		if end-st <= 4 {
			i++
			continue
		}

		// bad situations (*** means intersection)
		// st ... w.pos *** end ... w.pos+(iend-done)
		// w.pos ... st *** w.pos+(iend-done) ... end

		if q := end - int(w.pos); q > 0 {
			end -= q
			iend -= q
		}

		if q := int(w.pos) + (iend - done) - (st + len(w.block)); q > 0 {
			st += q
			ist += q
		}

		if end-st <= 4 {
			i++
			continue
		}

		if done < ist {
			w.appendLiteral(p, done, ist)
		}

		w.appendCopy(st, end)

		h = *(*uint32)(unsafe.Pointer(&p[i+1])) * 0x1e35a7bd >> w.hsh
		w.ht[h] = uint32(start + i + 1)

		i = iend
		done = iend
	}

	if done < len(p) {
		w.appendLiteral(p, done, len(p))

		done = len(p)
	}

	n, err := w.Writer.Write(w.b)
	w.written += int64(n)

	if err != nil {
		w.reset()
	}

	if n != len(w.b) {
		return 0, err
	}

	return done, err
}

func (w *Encoder) appendHeader(b []byte) []byte {
	b = append(b, Literal|Meta, MetaMagic|3, 't', 'l', 'z')

	b = append(b, Literal|Meta, MetaVer|byte(len(Version)))
	b = append(b, Version...)

	bs := 0
	for q := len(w.block); q != 1; q >>= 1 {
		bs++
	}

	b = append(b, Literal|Meta, MetaReset, byte(bs))

	return b
}

func (w *Encoder) appendLiteral(d []byte, s, e int) {
	w.b = w.appendTag(w.b, Literal, e-s)
	w.b = append(w.b, d[s:e]...)

	for s < e {
		n := copy(w.block[int(w.pos)&w.mask:], d[s:e])
		s += n
		w.pos += int64(n)
	}
}

func (w *Encoder) appendCopy(st, end int) {
	w.b = w.appendTag(w.b, Copy, end-st)
	w.b = w.appendOff(w.b, int(w.pos)-end)

	var n int
	for st < end {
		if st&w.mask < end&w.mask {
			n = copy(w.block[int(w.pos)&w.mask:], w.block[st&w.mask:end&w.mask])
		} else {
			n = copy(w.block[int(w.pos)&w.mask:], w.block[st&w.mask:])
		}
		st += n
		w.pos += int64(n)
	}
}

func (w *Encoder) appendTag(b []byte, tag byte, l int) []byte {
	switch {
	case l < Len1:
		return append(b, tag|byte(l))
	case l <= 0xff:
		return append(b, tag|Len1, byte(l))
	case l <= 0xffff:
		return append(b, tag|Len2, byte(l>>8), byte(l))
	case l <= 0xffff_ffff:
		return append(b, tag|Len4, byte(l>>24), byte(l>>16), byte(l>>8), byte(l))
	default:
		return append(b, tag|Len8, byte(l>>56), byte(l>>48), byte(l>>40), byte(l>>32), byte(l>>24), byte(l>>16), byte(l>>8), byte(l))
	}
}

func (w *Encoder) appendOff(b []byte, l int) []byte {
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
