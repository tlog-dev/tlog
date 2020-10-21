package tlog

import (
	"sync"
	"unsafe"
)

/*
	Format

	It's intended to be compact, fast encodable and resources effective.

	Stream consists of Records. Record consists of fields.

	Field is some kind of tag-len-value but optimized for compactness.

	Record is terminated by `0` tag.

	Record can be skipped only field by field until `0` got.

	Tags are:

	0 - end of record
	r - Record Type
	s - Span ID
	t - Wall Time
	i - Log Level (importance)
	l - Location ID (Program Counter, maybe compressed)
	m - Message (or name, depending on Type)
	f - additional user defined Field data
	v - Observation

	Encoding

	Some tags are encoded with len or value embedded.

	0 is '\0' byte.

	r is 0b0xxx_xxxx where xxx_xxxx is 7 least significant bits of ascii printable character.

	i is 0b1001_xxxx where xxxx is int4 log level.

	s is 0b1010_xxxx where xxxx is ID len - 1.

	m is 0b1011_0xxx where xxx is len_strlen - 1
		followed by 2 bytes strnum
		if len_strlen != 111
			followed by <len of strlen> bytes of strlen
			followed by strlen bytes of string

	f is 0b1000_0000

	l is 0b1000_0000

	t is 0b1000_0001 followed by 8 bytes unix nano time.


	Bits
	0	0000	4	0100	8	1000	c	1100
	1	0001	5	0101	9	1001	c	1101
	2	0010	6	0110	a	1010	d	1110
	3	0011	7	0111	b	1011	f	1111
*/

// taken from runtime/internal/sys
const PtrSize = 4 << (^uintptr(0) >> 63) // unsafe.Sizeof(uintptr(0)) but an ideal const

const (
	idbits  = 14
	idmask  = 1<<idbits - 1
	idbytes = 2

	ssbits = 14
	ssmask = 1<<ssbits - 1
)

type (
	encoder struct {
		mu sync.Mutex

		ls map[PC]int

		id [1 << idbits]ID

		ss [1 << ssbits]ssval

		b []byte
	}

	idpref [2]byte

	ssval struct {
		h    uintptr
		pref [12]byte
	}

	bwr struct {
		b   bufWriter
		buf [128 - unsafe.Sizeof([]byte{})]byte
	}

	bufWriter []byte
)

var spaces = []byte("                                                                                                                                                ")

var bufPool = sync.Pool{New: func() interface{} { w := &bwr{}; w.b = w.buf[:]; return w }}

func ev(l *Logger, id ID, tm int64, pc PC, tp Type, lv Level, fmt string, args []interface{}, kv KVs) {
	if l == nil {
		return
	}

	e := &l.enc

	defer e.mu.Unlock()
	e.mu.Lock()

	b := e.b[:cap(e.b)]
	if len(b) < 128 {
		b = make([]byte, 128)
	}

	i := 0

	if id != (ID{}) {
		b = appendID(e, b[:i], id, tp == 'f')
		i = len(b)
		b = b[:cap(b)]
	}

	if tp != 0 {
		b[i] = byte(tp) &^ 0x80
		i++
	}

	if lv != 0 {
		b[i] = 0b1001_0000 | byte(lv)&0x0f
		i++
	}

	if tm != 0 {
		b[i] = 0b1000_0001
		i++

		b[i] = byte(tm >> 56)
		i++
		b[i] = byte(tm >> 48)
		i++
		b[i] = byte(tm >> 40)
		i++
		b[i] = byte(tm >> 32)
		i++
		b[i] = byte(tm >> 24)
		i++
		b[i] = byte(tm >> 16)
		i++
		b[i] = byte(tm >> 8)
		i++
		b[i] = byte(tm)
		i++
	}

	b = b[:i]

	// be ready to b is not long enough from here

	if pc != 0 {
		// TODO
	}

	if fmt != "" || len(args) != 0 {
		b = appendMessage(e, b, fmt, args)
	}

	b = appendKVs(e, b, kv)

	e.b = append(b, 0)

	_, _ = l.Writer.Write(e.b)
}

func appendID(e *encoder, b []byte, id ID, clear bool) []byte {
	pref := *(*uint)(unsafe.Pointer(&id))
	pref &= idmask

	var short byte
	if e.id[pref] == id {
		short = idbytes
	} else {
		short = byte(len(id))

		e.id[pref] = id
	}

	b = append(b, 0b1010_0000|(short-1))
	b = append(b, id[:short]...)

	return b
}

func appendKVs(enc *encoder, b []byte, kvs KVs) []byte {
	for _, kv := range kvs {
		_ = kv
	}

	return b
}

func appendMessage(e *encoder, b []byte, fmt string, args []interface{}) []byte {
	b = append(b,
		0,    // tag
		0, 0, // strnum
		0, // strlen
	)

	st := len(b)

	if len(args) == 0 {
		b = append(b, fmt...)
	} else if fmt != "" {
		b = AppendPrintf(b, fmt, args...)
	} else {
		b = AppendPrintln(b, args...)
	}

	h := memhash(&b[st], 0, len(b)-st)

	b[st-3], b[st-2] = byte(h>>8), byte(h)

	cv := ssval{h: h}
	copy(cv.pref[:], b[st:])
	if e.ss[h&ssmask] == cv {
		b[st-4] = 0b1011_0111

		return b[:st-1]
	} else {
		e.ss[h&ssmask] = cv
	}

	if ml := len(b) - st; ml <= 0xff {
		b[st-4] = 0b1011_0000 | 0
		b[st-1] = byte(ml)
	} else {
		overflow := 1 // we already reserved 1 byte for len. this is the second
		if ml > 0xffff {
			overflow++ // third
		}
		if ml > 0xffffff {
			overflow++ // fourth
		}
		if ml > 0xffffffff {
			overflow++ // fifth
		}

		b[st-4] = 0b1011_0000 | byte(overflow)

		b = append(b, 0, 0, 0, 0)
		copy(b[st+overflow:], b[st:])

		b = b[:st+overflow+ml]

		for overflow >= -1 {
			b[st+overflow] = byte(ml)
			overflow--
			ml >>= 8
		}
	}

	return b
}

// Getbuf gets bytes buffer from a pool to reduce gc pressure.
// buffer is at least 100 bytes long.
// Buffer must be returned after used. Usage:
//     b, wr := tlog.Getbuf()
//     defer wr.Ret(&b)
//
//     b = append(b[:0], ...)
func Getbuf() (_ bufWriter, wr *bwr) { //nolint:golint
	wr = bufPool.Get().(*bwr)
	return wr.b, wr
}

func (wr *bwr) Ret(b *bufWriter) {
	wr.b = *b
	bufPool.Put(wr)
}

func (w *bufWriter) Write(p []byte) (int, error) {
	*w = append(*w, p...)
	return len(p), nil
}

func (w *bufWriter) NewLine() {
	l := len(*w)
	if l == 0 || (*w)[l-1] != '\n' {
		*w = append(*w, '\n')
	}
}
