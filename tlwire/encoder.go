package tlwire

import (
	"fmt"
	"net/netip"
	"time"

	"nikand.dev/go/cbor"
	"nikand.dev/go/hacked/htime"
)

type (
	Encoder struct {
		cbor.Emitter

		custom encoders
	}
)

func (e *Encoder) AppendKey(b []byte, key string) []byte {
	b = e.AppendTag(b, String, len(key))
	return append(b, key...)
}

func (e *Encoder) AppendKeyString(b []byte, k, v string) []byte {
	b = e.AppendTag(b, String, len(k))
	b = append(b, k...)

	b = e.AppendTag(b, String, len(v))
	b = append(b, v...)

	return b
}

func (e *Encoder) AppendKeyInt(b []byte, k string, v int) []byte {
	b = e.AppendTag(b, String, len(k))
	b = append(b, k...)
	return e.AppendInt(b, v)
}

func (e *Encoder) AppendKeyUint(b []byte, k string, v uint) []byte {
	b = e.AppendTag(b, String, len(k))
	b = append(b, k...)
	return e.AppendTag64(b, Int, uint64(v))
}

func (e *Encoder) AppendKeyInt64(b []byte, k string, v int64) []byte {
	b = e.AppendTag(b, String, len(k))
	b = append(b, k...)

	if v < 0 {
		return e.AppendTag64(b, Neg, uint64(-v)+1)
	}

	return e.AppendTag64(b, Int, uint64(v))
}

func (e *Encoder) AppendKeyUint64(b []byte, k string, v uint64) []byte {
	b = e.AppendTag(b, String, len(k))
	b = append(b, k...)
	return e.AppendTag64(b, Int, v)
}

func (e *Encoder) AppendError(b []byte, err error) []byte {
	b = append(b, byte(Semantic|Error))

	if err == nil {
		return append(b, byte(Special|Nil))
	}

	return e.AppendString(b, err.Error())
}

func (e *Encoder) AppendTime(b []byte, t time.Time) []byte {
	b = append(b, byte(Semantic|Time))
	return e.AppendInt64(b, t.UnixNano())
}

func (e *Encoder) AppendTimeTZ(b []byte, t time.Time) []byte {
	b = append(b, byte(Semantic|Time))
	b = append(b, byte(Map|2))

	b = e.AppendKeyInt64(b, "t", t.UnixNano())

	b = e.AppendKey(b, "z")
	b = append(b, byte(Array|2))

	n, off := t.Zone()
	b = e.AppendString(b, n)
	b = e.AppendInt(b, off)

	return b
}

func (e *Encoder) AppendTimeMonoTZ(b []byte, t time.Time) []byte {
	b = append(b, byte(Semantic|Time))

	mono := htime.MonotonicOf(t)
	fields := 2

	if mono != 0 {
		fields++
	}

	b = append(b, byte(Map)|byte(fields))

	b = e.AppendKeyInt64(b, "t", t.UnixNano())

	if mono != 0 {
		b = e.AppendKeyInt64(b, "m", mono)
	}

	b = e.AppendKey(b, "z")
	b = append(b, byte(Array|2))

	n, off := t.Zone()
	b = e.AppendString(b, n)
	b = e.AppendInt(b, off)

	return b
}

func (e *Encoder) AppendTimestamp(b []byte, t int64) []byte {
	b = append(b, byte(Semantic|Time))
	return e.AppendInt64(b, t)
}

func (e *Encoder) AppendDuration(b []byte, d time.Duration) []byte {
	b = append(b, byte(Semantic|Duration))
	return e.AppendInt64(b, d.Nanoseconds())
}

func (e *Encoder) AppendAddr(b []byte, a netip.Addr) []byte {
	b = append(b, byte(Semantic|NetAddr))

	if !a.IsValid() {
		return append(b, byte(Special|Nil))
	}

	b = e.AppendTag(b, String, 0)
	st := len(b)
	b = a.AppendTo(b)
	b = e.InsertLen(b, st, len(b)-st)

	return b
}

func (e *Encoder) AppendAddrPort(b []byte, a netip.AddrPort) []byte {
	b = append(b, byte(Semantic|NetAddr))

	if !a.IsValid() {
		return append(b, byte(Special|Nil))
	}

	b = e.AppendTag(b, String, 0)
	st := len(b)
	b = a.AppendTo(b)
	b = e.InsertLen(b, st, len(b)-st)

	return b
}

func (e *Encoder) AppendFormat(b []byte, format string, args ...interface{}) []byte {
	b = append(b, byte(String))
	st := len(b)

	if format == "" {
		b = fmt.Append(b, args...)
	} else {
		b = fmt.Appendf(b, format, args...)
	}

	l := len(b) - st

	return e.InsertLen(b, st, l)
}

// InsertLen inserts length l before value starting at st copying l bytes forward if needed.
// It is created for AppendFormat because we don't know the final message length.
// But it can be also used in other similar situations.
func (e *Encoder) InsertLen(b []byte, st, l int) []byte {
	return e.Emitter.InsertLen(b, Tag(b[st-1])&TagMask, st, 0, l)
}
