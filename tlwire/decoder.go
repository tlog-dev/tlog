package tlwire

import (
	"net/netip"
	"time"

	"nikand.dev/go/cbor"
)

type (
	Decoder struct {
		cbor.Iterator
	}
)

func (d *Decoder) Time(p []byte, st int) (t time.Time, i int) {
	if Tag(p[st]) != Semantic|Time {
		panic("not a time")
	}

	tag, sub, i := d.Tag(p, st+1)

	if tag == Int {
		return time.Unix(0, sub), i
	}

	if tag != Map || sub == -1 {
		panic("unsupported time")
	}

	var (
		k     []byte
		ts    int64
		tzN   []byte
		tzOff int64
	)

	for range sub {
		k, i = d.Bytes(p, i)

		switch string(k) {
		case "t":
			ts, i = d.Signed(p, i)
		case "z":
			if Tag(p[i]) != Array|2 {
				panic("unsupported time zone")
			}
			i++

			tzN, i = d.Bytes(p, i)
			tzOff, i = d.Signed(p, i)
		default:
			i = d.Skip(p, i)
		}
	}

	if ts != 0 {
		t = time.Unix(0, ts)
	}

	if tzN != nil || tzOff != 0 {
		l := time.FixedZone(string(tzN), int(tzOff))
		t = t.In(l)
	}

	return
}

func (d *Decoder) Timestamp(p []byte, st int) (ts int64, i int) {
	if Tag(p[st]) != Semantic|Time {
		panic("not a time")
	}

	tag, sub, i := d.Tag(p, st+1)

	if tag == Int {
		return sub, i
	}

	if tag != Map || sub == -1 {
		panic("unsupported time")
	}

	var k []byte

	for range sub {
		k, i = d.Bytes(p, i)

		switch string(k) {
		case "t":
			ts, i = d.Signed(p, i)
		default:
			i = d.Skip(p, i)
		}
	}

	return
}

func (d *Decoder) Duration(p []byte, st int) (dr time.Duration, i int) {
	if Tag(p[st]) != Semantic|Duration {
		panic("not a duration")
	}

	tag, v, i := d.Tag(p, st+1)

	if tag != Int && tag != Neg {
		panic("unsupported duration")
	}

	if tag == Neg {
		v = -v
	}

	return time.Duration(v), i
}

func (d *Decoder) Addr(p []byte, st int) (a netip.Addr, ap netip.AddrPort, i int, err error) {
	if Tag(p[st]) != Semantic|NetAddr {
		panic("not an address")
	}

	tag, l, i := d.Tag(p, st+1)
	if tag == Special && d.Simple(p, st+1) == Nil {
		return
	}
	if tag != String {
		panic("unsupported address encoding")
	}

	ab := p[i : i+int(l)]
	i += int(l)

	err = a.UnmarshalText(ab)
	if err != nil {
		err = ap.UnmarshalText(ab)
	}
	if err != nil {
		return a, ap, st, err
	}

	return
}
