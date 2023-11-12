package convert

import (
	_ "embed"
	"errors"
	"io"
	"time"

	"github.com/nikandfor/hacked/hfmt"
	"tlog.app/go/loc"
	"tlog.app/go/tlog"
	"tlog.app/go/tlog/tlio"
	"tlog.app/go/tlog/tlwire"
)

type (
	Web struct {
		io.Writer

		EventTimeFormat string
		TimeFormat      string

		PickTime    bool
		PickCaller  bool
		PickMessage bool

		d tlwire.Decoder
		l Logfmt
		j JSON
		c *tlog.ConsoleWriter

		s []tlog.ID
		m []byte

		time, last []byte

		bb, b, ls []byte
	}
)

//go:embed webstyles.css
var webstyles []byte

func NewWeb(w io.Writer) *Web {
	ww := &Web{
		Writer:          w,
		EventTimeFormat: "2006-01-02 15:04:05.000",
		TimeFormat:      "2006-01-02 15:04:05.000",
		PickTime:        true,
		//PickCaller:      true,
		PickMessage: true,

		c: tlog.NewConsoleWriter(nil, 0),
	}

	ww.c.Colorize = false

	return ww
}

func (w *Web) Write(p []byte) (i int, err error) {
	if w.last == nil {
		w.bb = w.buildHeader(w.bb[:0])
	}

more:
	tag, els, i := w.d.Tag(p, i)
	if tag != tlwire.Map {
		return i, errors.New("map expected")
	}

	var t time.Time
	var c loc.PC
	var ek tlog.EventKind
	var m []byte

	var k []byte
	var sub int64

	for el := 0; els == -1 || el < int(els); el++ {
		if els == -1 && w.d.Break(p, &i) {
			break
		}

		k, i = w.d.Bytes(p, i)
		if len(k) == 0 {
			return 0, errors.New("empty key")
		}

		st := i

		tag, sub, i = w.d.Tag(p, i)
		if tag != tlwire.Semantic {
			w.b, i = w.appendPair(w.b, p, k, st)
			continue
		}

		switch {
		case w.PickTime && sub == tlwire.Time && string(k) == tlog.KeyTimestamp:
			t, i = w.d.Time(p, st)
		case w.PickCaller && sub == tlwire.Caller && string(k) == tlog.KeyCaller && c == 0:
			c, i = w.d.Caller(p, st)
		case w.PickMessage && sub == tlog.WireMessage && string(k) == tlog.KeyMessage:
			m, i = w.d.Bytes(p, i)
		case sub == tlog.WireID:
			var id tlog.ID
			_ = id.TlogParse(p, st)

			w.s = append(w.s, id)
			w.b, i = w.appendPair(w.b, p, k, st)
		case sub == tlog.WireEventKind && string(k) == tlog.KeyEventKind:
			_ = ek.TlogParse(p, st)

			w.b, i = w.appendPair(w.b, p, k, st)
		case sub == tlog.WireLabel:
			w.ls, i = w.appendPair(w.ls, p, k, st)
		default:
			w.b, i = w.appendPair(w.b, p, k, st)
		}
	}

	w.bb = w.buildEvent(w.bb, t, c, m)

	w.s = w.s[:0]
	w.b = w.b[:0]
	w.ls = w.ls[:0]

	if i < len(p) {
		goto more
	}

	bb := w.bb
	w.bb = w.bb[:0]

	_, err = w.Writer.Write(bb)
	if err != nil {
		return 0, err
	}

	return len(p), nil
}

func (w *Web) Close() error {
	w.bb = w.buildFooter(w.bb[:0])
	_, err := w.Writer.Write(w.bb)

	e := tlio.Close(w.Writer)
	if err == nil {
		err = e
	}

	return err
}

func (w *Web) buildHeader(b []byte) []byte {
	b = append(b, `<html>
<head>
</head>
<style>`...)

	b = append(b, webstyles...)

	b = append(b, `</style>
<body>
<table class=events>
`...)

	return b
}

func (w *Web) buildFooter(b []byte) []byte {
	b = append(b, `</table>
</body>
</html>
`...)

	return b
}

func (w *Web) buildEvent(b []byte, t time.Time, c loc.PC, m []byte) []byte {
	b = append(b, `<tr class="event`...)

	for _, s := range w.s {
		b = hfmt.Appendf(b, " id%08v", s)
	}

	b = append(b, "\">\n"...)

	if w.PickTime {
		b = w.buildTime(b, t)
	}

	if w.PickCaller {
		b = hfmt.Appendf(b, "<td class=caller>%s</td>\n", c.String())
	}

	if w.PickMessage {
		b = hfmt.Appendf(b, "<td class=msg>%s</td>\n", m)
	}

	b = hfmt.Appendf(b, "<td class=kvs>%s</td>\n", w.b)

	b = append(b, "</tr>\n"...)

	return b
}

func (w *Web) buildTime(b []byte, t time.Time) []byte {
	w.time = t.AppendFormat(w.time[:0], w.TimeFormat)
	c := common(w.time, w.last)

	b = append(b, "<td class=ev-time>"...)

	if c != 0 {
		b = hfmt.Appendf(b, `<span class=ev-time-pref>%s</span>`, w.time[:c])
	}
	if c != len(w.time) {
		b = hfmt.Appendf(b, `<span class=ev-time-suff>%s</span>`, w.time[c:])
	}

	b = append(b, "</td>\n"...)

	w.last, w.time = w.time, w.last

	return b
}

func (w *Web) appendPair(b, p, k []byte, st int) (_ []byte, i int) {
	b = hfmt.Appendf(b, `<div class=kv><span class=key>%s=</span><div class=val>`, k)

	b, i = w.c.ConvertValue(b, p, st, 0)

	b = append(b, "</div></div>\n"...)

	return b, i
}

func common(a, b []byte) (n int) {
	for n < len(b) && a[n] == b[n] {
		n++
	}

	return
}
