package tlwire

import (
	"fmt"
	"sync"

	"github.com/nikandfor/loc"
)

var (
	locmu    sync.Mutex
	loccache = map[loc.PC][]byte{}
)

func (e Encoder) AppendPC(b []byte, pc loc.PC) []byte {
	b = append(b, Semantic|Caller)

	if pc == 0 {
		return append(b, Special|Nil)
	}

	return e.AppendUint64(b, uint64(pc))
}

func (e Encoder) AppendPCs(b []byte, pcs loc.PCs) []byte {
	b = append(b, Semantic|Caller)

	if pcs == nil {
		return append(b, Special|Nil)
	}

	b = e.AppendTag(b, Array, len(pcs))

	for _, pc := range pcs {
		b = e.AppendUint64(b, uint64(pc))
	}

	return b
}

func (e Encoder) AppendCaller(b []byte, pc loc.PC) []byte {
	b = append(b, Semantic|Caller)

	return e.appendPC(b, pc)
}

func (e Encoder) AppendCallers(b []byte, pcs loc.PCs) []byte {
	b = append(b, Semantic|Caller)
	b = e.AppendTag(b, Array, len(pcs))

	for _, pc := range pcs {
		b = e.appendPC(b, pc)
	}

	return b
}

func (e Encoder) appendPC(b []byte, pc loc.PC) []byte {
	if pc == 0 {
		return append(b, Special|Nil)
	}

	locmu.Lock()
	c, ok := loccache[pc]
	locmu.Unlock()

	if ok {
		return append(b, c...)
	}

	fe := pc.FuncEntry()

	st := len(b)

	l := byte(4)
	if fe != pc {
		l++
	}

	b = append(b, Map|l)

	b = e.AppendString(b, "p")
	b = e.AppendUint64(b, uint64(pc))

	name, file, line := pc.NameFileLine()

	b = e.AppendString(b, "n")
	b = e.AppendString(b, name)

	b = e.AppendString(b, "f")
	b = e.AppendString(b, file)

	b = e.AppendString(b, "l")
	b = e.AppendInt(b, line)

	if fe != pc {
		b = e.AppendString(b, "e")
		b = e.AppendUint64(b, uint64(fe))
	}

	c = make([]byte, len(b)-st)
	copy(c, b[st:])

	locmu.Lock()
	loccache[pc] = c
	locmu.Unlock()

	return b
}

func (d Decoder) Caller(p []byte, st int) (pc loc.PC, i int) {
	if p[st] != Semantic|Caller {
		panic("not a caller")
	}

	tag, sub, i := d.Tag(p, st+1)

	if tag == Int || tag == Map {
		return d.caller(p, st+1)
	}

	if tag == Special && sub == Nil {
		return
	}

	if tag != Array {
		panic(fmt.Sprintf("unsupported caller tag: %x", tag))
	}

	if sub == 0 {
		return
	}

	pc, i = d.caller(p, i)

	for el := 1; el < int(sub); el++ {
		_, i = d.caller(p, i)
	}

	return
}

func (d Decoder) Callers(p []byte, st int) (pc loc.PC, pcs loc.PCs, i int) {
	if p[st] != Semantic|Caller {
		panic("not a caller")
	}

	tag, sub, i := d.Tag(p, st+1)

	switch {
	case tag == Int, tag == Map:
		pc, i = d.caller(p, st+1)
		return
	case tag == Array:
	case tag == Special && sub == Nil:
		return
	default:
		panic(fmt.Sprintf("unsupported caller tag: %x", tag))
	}

	if sub == 0 {
		return
	}

	pcs = make(loc.PCs, sub)

	for el := 0; el < int(sub); el++ {
		pcs[el], i = d.caller(p, i)
	}

	pc = pcs[0]

	return
}

func (d Decoder) caller(p []byte, st int) (pc loc.PC, i int) {
	i = st

	tag, sub, i := d.Tag(p, i)

	if tag == Int {
		pc = loc.PC(sub)

		if pc != 0 && !loc.Cached(pc) {
			loc.SetCache(pc, "_", ".", 0)
		}

		return
	}

	var v uint64
	var k []byte
	var name, file []byte
	var line int

	for el := 0; el < int(sub); el++ {
		k, i = d.Bytes(p, i)

		switch string(k) {
		case "p":
			v, i = d.Unsigned(p, i)

			pc = loc.PC(v)
		case "l":
			v, i = d.Unsigned(p, i)

			line = int(v)
		case "n":
			name, i = d.Bytes(p, i)
		case "f":
			file, i = d.Bytes(p, i)
		default:
			i = d.Skip(p, i)
		}
	}

	if pc == 0 {
		return
	}

	loc.SetCacheBytes(pc, name, file, line)

	return
}
