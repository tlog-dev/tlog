package processor

import (
	"errors"
	"io"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/wire"
)

// (select(.m == "peer_syncer") | .s) as $sid

type (
	Writer struct {
		io.Writer

		NonTraced bool
		MaxDepth  int

		d wire.Decoder

		span []string

		id map[tlog.ID]int
	}
)

func New(w io.Writer, span ...string) *Writer {
	return &Writer{
		Writer: w,
		span:   span,
		id:     make(map[tlog.ID]int),
	}
}

func (w *Writer) Write(p []byte) (i int, err error) {
	tag, els, i := w.d.Tag(p, i)
	if tag != wire.Map {
		return i, errors.New("map expected")
	}

	var sid, par tlog.ID
	var name []byte

	var k []byte
	var sub int64
	for el := 0; els == -1 || el < int(els); el++ {
		if els == -1 && w.d.Break(p, &i) {
			break
		}

		k, i = w.d.String(p, i)
		tag, sub, _ = w.d.Tag(p, i)

		if tag != wire.Semantic {
			i = w.d.Skip(p, i)
			continue
		}

		switch {
		case sub == tlog.WireID && string(k) == tlog.KeySpan:
			i = sid.TlogParse(&w.d, p, i)
		case sub == tlog.WireID && string(k) == tlog.KeyParent:
			i = par.TlogParse(&w.d, p, i)
		case sub == tlog.WireMessage && string(k) == tlog.KeyMessage:
			_, _, i = w.d.Tag(p, i)
			name, i = w.d.String(p, i)
		default:
			i = w.d.Skip(p, i)
		}
	}

	var selected bool

	if sid == (tlog.ID{}) {
		selected = w.NonTraced
	} else {
		if name != nil {
			for _, span := range w.span {
				if span == string(name) {
					w.id[sid] = 0
				}

				if l := len(span); l <= len(tlog.ID{})*2 && sid.StringFull()[:l] == span {
					w.id[sid] = 0
				}
			}
		}

		if par != (tlog.ID{}) {
			if d, ok := w.id[par]; ok && d < w.MaxDepth {
				w.id[sid] = d + 1
			}
		}

		_, selected = w.id[sid]

		if !selected && par != (tlog.ID{}) {
			_, selected = w.id[par]
		}
	}

	if w.Writer == nil {
		return len(p), nil
	}

	if !selected {
		return len(p), nil
	}

	//	var e wire.Encoder
	//	f := e.AppendKeyValue(nil, "selected", selected)
	//	q := convert.Set(nil, p, f)

	return w.Writer.Write(p)
}
