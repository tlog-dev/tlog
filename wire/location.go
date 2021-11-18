package wire

import (
	"sync"

	"github.com/nikandfor/loc"
)

var (
	locmu    sync.Mutex
	loccache = map[loc.PC][]byte{}
)

func (e *Encoder) AppendPC(b []byte, pc loc.PC) []byte {
	b = append(b, Semantic|Caller)

	return e.appendPC(b, pc)
}

func (e *Encoder) AppendPCs(b []byte, pcs loc.PCs) []byte {
	b = append(b, Semantic|Caller)
	b = e.AppendTag(b, Array, int64(len(pcs)))

	for _, pc := range pcs {
		b = e.appendPC(b, pc)
	}

	return b
}

func (e *Encoder) appendPC(b []byte, pc loc.PC) []byte {
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
