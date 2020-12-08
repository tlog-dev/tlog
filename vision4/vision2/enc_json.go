package tlog

import (
	"sync"
	"unsafe"
)

/*
	Format

	It's intended to be compact, fast encodable and resources effective.

	It's tag[-len][-value] set of fields. Len and Value presence is specified for each tag separately.

	Stream consists of Records each of which consists of fields. Record is terminated by `0` tag.
	Record can be skipped only field by field until `0` got.

	Tags are:

	0 - end of record
	r - Record Type
	s - Span ID
	t - Wall Time
	i - Log Level (importance)
	l - Location ID (Program Counter, maybe compressed)
	m - Message (or name, depending on Type)
	f - additional user defined Field
	v - Observation.

	Encoding

	Some tags are encoded with len or value embedded.

	0 is '\0' byte.

	r is 0b0xxx_xxxx where xxx_xxxx is 7 least significant bits of ascii printable character.

	i is 0b1001_xxxx where xxxx is int4 log level.

	s is 0b1010_xxxx where xxxx is ID len.

	m is 0b1011_xyyy where x is shortid is present
		if x == 0 than yzz is number of bytes of len field.
		if x == 1 than

	f is 0b1000_0000

	l is 0b1000_0000

	v is 0b1000_0000

	t is 0b1000_0001 followed by 8 bytes unix nano time.
*/

type (
	encoder struct {
		mu sync.RWMutex
	}

	bwr struct {
		b   []byte
		buf [128 - unsafe.Sizeof([]byte{})]byte
	}
)

var bufPool = sync.Pool{New: func() interface{} { w := &bwr{}; w.b = w.buf[:]; return w }}

func ev(l *Logger, id ID, d int8, tp Type, lv Level, fmt string, args []interface{}, kv KV) {
	if l == nil {
		return
	}

	var tm int64
	if l.AddTime {
		tm = now().UnixNano()
	}

	var pc PC
	if l.AddCaller && d != -1 {
		pc = Caller(2 + d)
	}

	b, wr := Getbuf()
	defer wr.Ret(&b)

	defer l.enc.mu.Unlock()
	l.enc.mu.Lock()

	// b is at least 64 bytes long

	i := 0

	if id != (ID{}) {
		b[i] = 0b1010_0000 | len(id)
		i++
		i += copy(b[i:], id[:])
	}

	if tp != 0 {
		b[i] = byte(tp) &^ 0x80
		i++
	}

	if lv != 0 {
		b[i] = 0b1001_0000 | byte(lv)&0x0f
		i++
	}

	if l.AddTime {
		b[i] = 0b1000_0001
		i++
		binary.BigEndian.PutUint64(b[i:], uint64(tm))
		i += 8
	}

	// be ready to b is not long enough from here

	if l.AddCaller && d != -1 {
		// TODO
	}

	b, i = appendMessage(b, i, fmt, args)
}

func appendMessage(b []byte, i int, fmt string, args []interface{}) ([]byte, int) {
	if fmt == "" && args == nil {
		return b, i
	}

	b = append(b,
		0, // tag // TODO
		0, // len placeholder
	)

	st := len(b)

	if args == nil {
		b = append(b, fmt)
	} else if fmt != "" {
		b = AppendPrintf(b, fmt, args...)
	} else {
		b = AppendPrintln(b, args...)
	}

	if ml := len(b) - st; ml <= 0xff {
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

		b = append(b, 0, 0, 0, 0)
		copy(b[st+overflow:], b[st:])

		b = b[:st+overflow+ml]

		for overflow >= -1 {
			b[st+overflow] = byte(ml)
			overflow--
			ml >>= 8
		}
	}

	return b, i
}

// Getbuf gets bytes buffer from a pool to reduce gc pressure.
// Buffer must be returned after used. Usage:
//     b, wr := tlog.Getbuf()
//     defer wr.Ret(&b)
//
//     b = append(b[:0], ...)
func Getbuf() (_ bufWriter, wr *bwr) { //nolint:golint
	wr = bufPool.Get().(*bwr)
	return wr.b[:0], wr
}

func (wr *bwr) Ret(b *bufWriter) {
	wr.b = *b
	bufPool.Put(wr)
}
