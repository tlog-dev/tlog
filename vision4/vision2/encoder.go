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
	m - Message
	n - Name (alias for m)
	f - additional user defined Field data
	v - Observation

	Encoding

	Some tags are encoded with len or value embedded.

	0 is 0x00 byte.

	i is 0b0001_xxxx where xxxx is int4 log level.

	s is 0b0010_xxxx where xxxx is ID len - 1.

	l is 0b0011_yxxx where y if first 3 fields are name, file, line; xxx is location num len.
		followed by location_num

	m is 0b10xx_xxxx where x_xxxx is str_len.
		followed by str_len varlen encoded if xx_xxxx is all 1s
		followed by string.

	f is 0b11xx_xxxx where xx_xxxx is total_len.
		followed by total_len varlen encoded of xx_xxxx is all 1s
		followed by key_len in varint
		followed by key
		followed by value_type
		followed by value

	t is 0b0000_0001 followed by 8 bytes unix nano time.

	r is 0b0000_0010 followed by Type byte

	v is 0b0000_0100 followed by float64 or
	     0b0000_0101 followed by int64


	Field value types

	3 Lower bits are the same as in protobuf


	Bits
	0	0000	4	0100	8	1000	c	1100
	1	0001	5	0101	9	1001	d	1101
	2	0010	6	0110	a	1010	e	1110
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

		b []byte
	}

	bwr struct {
		b   bufWriter
		buf [128 - unsafe.Sizeof([]byte{})]byte
	}
)

var spaces = []byte("                                                                                                                                                ")

var bufPool = sync.Pool{New: func() interface{} { w := &bwr{}; w.b = w.buf[:]; return w }}

func ev(l *Logger, id ID, tm int64, pc PC, tp Type, lv Level, fmt string, args []interface{}, kvs []interface{}) {
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
		b = appendID(e, b[:i], id)
		i = len(b)
		b = b[:cap(b)]
	}

	if tp != 0 {
		b[i] = 0b0000_0010
		i++
		b[i] = byte(tp)
		i++
	}

	if lv != 0 {
		b[i] = 0b0001_0000 | byte(lv)&0x0f
		i++
	}

	if tm != 0 {
		b[i] = 0b0000_0001
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

	b = appendKVs(e, b, kvs)

	e.b = append(b, 0) // end of record

	_, _ = l.Writer.Write(e.b)
}

func appendID(e *encoder, b []byte, id ID) []byte {
	b = append(b, 0b0010_1111)
	b = append(b, id[:]...)

	return b
}

func appendKVs(enc *encoder, b []byte, kvs []interface{}) []byte {
	i := 0
	for i < len(kvs) {
		k := kvs[i].(string)
		i++

		v := kvs[i]
		i++

		st := len(b) + 1

		b = append(b,
			0b1100_0000,            // tag
			0, 0, 0, 0, 0, 0, 0, 0, // key_len
		)

		b = appendVarlen(b[:st], len(k))
		b = append(b, k...)

		switch v := v.(type) {
		case string:
			b = append(b, 's')
			b = appendVarlen(b, len(v))
			b = append(b, v...)
		case int, int64, int32, int16, int8:
			var q int64
			switch v := v.(type) {
			case int:
				q = int64(v)
			case int64:
				q = int64(v)
			case int32:
				q = int64(v)
			case int16:
				q = int64(v)
			case int8:
				q = int64(v)
			}

			b = appendInt(b, q)
		case uint, uint64, uint32, uint16, uint8:
			var q uint64
			switch v := v.(type) {
			case uint:
				q = uint64(v)
			case uint64:
				q = uint64(v)
			case uint32:
				q = uint64(v)
			case uint16:
				q = uint64(v)
			case uint8:
				q = uint64(v)
			}

			b = appendUint(b, q)
		case float64:
			b = append(b, 'f')
		case float32:
			b = append(b, 'f')
		default:
			panic(v)
		}

		total := len(b) - st

		if total < 1<<6 {
			b[st-1] |= byte(total)

			continue
		}

		b[st-1] |= 1 << 6

		b = insertLen(b, st, total-(1<<6))
	}

	return b
}

func appendMessage(e *encoder, b []byte, fmt string, args []interface{}) []byte {
	b = append(b,
		0b1000_0000, // tag, lowest 6 bits are length
	)

	st := len(b)

	if len(args) == 0 {
		b = append(b, fmt...)
	} else if fmt != "" {
		b = AppendPrintf(b, fmt, args...)
	} else {
		b = AppendPrintln(b, args...)
	}

	l := len(b) - st

	if l < 1<<6 {
		b[st-1] |= byte(l)

		return b
	}

	b[st-1] |= byte(1 << 6)
	l -= 1 << 6

	b = insertLen(b, st, l)

	return b
}

func insertLen(b []byte, st, l int) []byte {
	sz := 0
	for q := l; q > 0; q >>= 7 {
		sz++
	}

	b = append(b, 0, 0, 0, 0, 0, 0, 0, 0)[:len(b)+sz]
	copy(b[st+sz:], b[st:])

	for l >= 0x80 {
		b[st] = byte(l) | 0x80
		l >>= 7
		st++
	}

	b[st] = byte(l) &^ 0x80

	return b
}

func moveOverflow(b []byte, st, l, i int) []byte {
	overflow := 1 // we already reserved 1 byte for len. this is the second
	if l > 0xffff {
		overflow++ // third
	}
	if l > 0xffffff {
		overflow++ // fourth
	}
	if l > 0xffffffff {
		overflow++ // fifth
	}

	b[i] |= byte(overflow)

	b = append(b, 0, 0, 0, 0)
	copy(b[st+overflow:], b[st:])

	b = b[:st+overflow+l]

	for overflow >= -1 {
		b[st+overflow] = byte(l)
		overflow--
		l >>= 8
	}

	return b
}

func appendInt(b []byte, v int64) []byte {
	b = append(b, 'i')
	return appendVarint(b, v)
}

func appendUint(b []byte, v uint64) []byte {
	b = append(b, 'u')
	return appendVaruint(b, v)
}

func appendVarlen(b []byte, v int) []byte {
	return appendVaruint(b, uint64(v))
}

func appendVarint(b []byte, v int64) []byte {
	return appendVaruint(b, uint64(v<<1)^uint64(v>>63))
}

func appendVaruint(b []byte, v uint64) []byte {
	i := len(b)
	b = append(b, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)

	for v >= 0x80 {
		b[i] = byte(v) | 0x80
		v >>= 7
		i++
	}

	b[i] = byte(v) &^ 0x80
	i++

	return b[:i]
}
