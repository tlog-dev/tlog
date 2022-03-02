package tlgraylog

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/syslog"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/nikandfor/loc"
	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/low"
	"github.com/nikandfor/tlog/wire"
)

type (
	Writer struct {
		io.Writer

		AppendKeySafe bool

		Hostname string

		TimeFormat string
		TimeZone   *time.Location

		d wire.Decoder

		b []byte
	}
)

func New(w io.Writer) *Writer {
	return &Writer{
		Writer: w,

		AppendKeySafe: true,
		Hostname:      tlog.Hostname(),
	}
}

func (w *Writer) Write(p []byte) (i int, err error) {
	tag, els, i := w.d.Tag(p, i)
	if tag != wire.Map {
		return i, errors.New("map expected")
	}

	defer func() {
		pn := recover()
		if pn == nil {
			return
		}

		fmt.Fprintf(os.Stderr, "panic: %v\n%s", pn, wire.Dump(p))

		panic(pn)
	}()

	b := w.b[:0]

	b = append(b, '{')

	b = append(b, `"version":"1.1","host":"`...)
	b = append(b, w.Hostname...)
	b = append(b, '"')

	var lvl tlog.LogLevel
	var msg []byte
	var kind tlog.EventKind

	var k []byte
	var sub int64
	for el := 0; els == -1 || el < int(els); el++ {
		if els == -1 && w.d.Break(p, &i) {
			break
		}

		b = append(b, ',')

		k, i = w.d.String(p, i)

		tag, sub, _ = w.d.Tag(p, i)

		ks := low.UnsafeBytesToString(k)
		switch {
		case sub == wire.Time && ks == tlog.KeyTime:
			var ts int64
			ts, i = w.d.Timestamp(p, i)

			b = append(b, `"timestamp":`...)
			b = strconv.AppendFloat(b, float64(ts)/float64(time.Second), 'f', 3, 64)

			continue
		case sub == wire.Caller && ks == tlog.KeyCaller:
			var pc loc.PC
			pc, i = w.d.Caller(p, i)

			_, file, line := pc.NameFileLine()

			b = append(b, `"file":"`...)
			b = append(b, file...)
			b = append(b, `","line":`...)
			b = strconv.AppendInt(b, int64(line), 10)

			continue
		case sub == tlog.WireMessage && ks == tlog.KeyMessage:
			_, _, i = w.d.Tag(p, i)

			msg, i = w.d.String(p, i)

			b = append(b, `"short_message":"`...)
			b = append(b, msg...)
			b = append(b, '"')

			continue
		case sub == tlog.WireLogLevel && ks == tlog.KeyLogLevel:
			i = lvl.TlogParse(&w.d, p, i)

			b = b[:len(b)-1] // unwrite comma

			continue
		case sub == tlog.WireLabels && ks == tlog.KeyLabels:
			var ls tlog.Labels
			i = ls.TlogParse(&w.d, p, i)

			for i, l := range ls {
				k, v := ls.Split(l)

				if i != 0 {
					b = append(b, ',')
				}

				b = append(b, `"_L_`...)
				b = append(b, k...)
				b = append(b, `":"`...)
				b = append(b, v...)
				b = append(b, `"`...)
			}

			continue
		case sub == tlog.WireEventKind && ks == tlog.KeyEventKind:
			_ = kind.TlogParse(&w.d, p, i)
		}

		b = append(b, '"')

		//	if len(k) == 0 || k[0] != '_' {
		b = append(b, '_')
		//	}

		if w.AppendKeySafe {
			b = low.AppendSafe(b, k)
		} else {
			b = append(b, k...)
		}

		b = append(b, '"', ':')

		b, i = w.convertValue(b, p, i, false)
	}

	if len(msg) == 0 {
		if kind == tlog.EventSpanFinish {
			b = append(b, `,"short_message":"_span_finish_"`...)
		} else {
			b = append(b, `,"short_message":"_no_message_"`...)
		}
	}

	{
		lvl := toSyslogLevel(lvl)

		b = append(b, `,"level":`...)
		b = strconv.AppendInt(b, int64(lvl), 10)
	}

	b = append(b, '}', '\n')

	w.b = b[:0]

	_, err = w.Writer.Write(b)
	if err != nil {
		return 0, err
	}

	return len(p), nil
}

func (w *Writer) convertValue(b, p []byte, st int, esc bool) (_ []byte, i int) {
	tag, sub, i := w.d.Tag(p, st)

	switch tag {
	case wire.Int:
		b = strconv.AppendUint(b, uint64(sub), 10)
	case wire.Neg:
		b = strconv.AppendInt(b, sub, 10)
	case wire.Bytes:
		if !esc {
			b = append(b, '"')
		}

		m := base64.StdEncoding.EncodedLen(int(sub))
		st := len(b)

		for st+m < cap(b) {
			b = append(b[:cap(b)], 0, 0, 0, 0)
		}

		b = b[:st+m]

		base64.StdEncoding.Encode(b[st:], p[i:])

		if !esc {
			b = append(b, '"')
		}

		i += int(sub)
	case wire.String:
		if !esc {
			b = append(b, '"')
		}

		b = low.AppendSafe(b, p[i:i+int(sub)])

		if !esc {
			b = append(b, '"')
		}

		i += int(sub)
	case wire.Array:
		if !esc {
			b = append(b, '"')
		}
		b = append(b, '[')

		for el := 0; sub == -1 || el < int(sub); el++ {
			if sub == -1 && w.d.Break(p, &i) {
				break
			}

			if el != 0 {
				b = append(b, ',')
			}

			b, i = w.convertValue(b, p, i, true)
		}

		b = append(b, ']')
		if !esc {
			b = append(b, '"')
		}
	case wire.Map:
		var k []byte

		if !esc {
			b = append(b, '"')
		}
		b = append(b, '{')

		for el := 0; sub == -1 || el < int(sub); el++ {
			if sub == -1 && w.d.Break(p, &i) {
				break
			}

			if el != 0 {
				b = append(b, ',')
			}

			k, i = w.d.String(p, i)

			if w.AppendKeySafe {
				b = low.AppendSafe(b, k)
			} else {
				b = append(b, k...)
			}

			b = append(b, ':')

			b, i = w.convertValue(b, p, i, true)
		}

		b = append(b, '}')
		if !esc {
			b = append(b, '"')
		}
	case wire.Semantic:
		switch sub {
		case wire.Time:
			var t time.Time
			t, i = w.d.Time(p, st)

			if w.TimeZone != nil {
				t = t.In(w.TimeZone)
			}

			if w.TimeFormat != "" {
				b = append(b, '"')
				b = t.AppendFormat(b, w.TimeFormat)
				b = append(b, '"')
			} else {
				b = strconv.AppendInt(b, t.UnixNano(), 10)
			}
		case tlog.WireID:
			var id tlog.ID
			i = id.TlogParse(&w.d, p, st)

			bst := len(b) + 1
			b = append(b, `"123456789_123456789_123456789_12"`...)

			id.FormatTo(b[bst:], 'x')
		case wire.Caller:
			var pc loc.PC
			var pcs loc.PCs
			pc, pcs, i = w.d.Callers(p, st)

			if pcs != nil {
				if !esc {
					b = append(b, '"')
				}
				b = append(b, '[')
				for i, pc := range pcs {
					if i != 0 {
						b = append(b, ',')
					}

					_, file, line := pc.NameFileLine()
					b = low.AppendPrintf(b, `"%v:%d"`, filepath.Base(file), line)
				}
				b = append(b, ']')
				if !esc {
					b = append(b, '"')
				}
			} else {
				_, file, line := pc.NameFileLine()

				b = low.AppendPrintf(b, `"%v:%d"`, filepath.Base(file), line)
			}
		default:
			b, i = w.convertValue(b, p, i, esc)
		}
	case wire.Special:
		switch sub {
		case wire.False:
			b = append(b, "0"...)
		case wire.True:
			b = append(b, "1"...)
		case wire.Nil, wire.Undefined:
			b = append(b, "0"...)
		case wire.Float64, wire.Float32, wire.Float16, wire.Float8:
			var f float64
			f, i = w.d.Float(p, st)

			b = strconv.AppendFloat(b, f, 'f', -1, 64)
		default:
			panic(sub)
		}
	}

	return b, i
}

func toSyslogLevel(l tlog.LogLevel) int {
	switch l {
	case tlog.Info:
		return int(syslog.LOG_INFO)
	case tlog.Warn:
		return int(syslog.LOG_WARNING)
	case tlog.Error:
		return int(syslog.LOG_ERR)
	default:
		if l >= tlog.Fatal {
			return int(syslog.LOG_CRIT)
		}

		return int(syslog.LOG_DEBUG)
	}
}
