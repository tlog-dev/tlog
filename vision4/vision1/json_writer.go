package tlog

import (
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"
)

type (
	JSONWriter struct {
		io.Writer

		mu sync.RWMutex
		ls map[PC]struct{}
	}
)

const digitsx = "0123456789abcdef"

var spaces = []byte("                                                                                                                                                ")

var _ Writer = &JSONWriter{}

func NewJSONWriter(w io.Writer) *JSONWriter {
	return &JSONWriter{
		Writer: w,
		ls:     make(map[PC]struct{}),
	}
}

func (w *JSONWriter) Write(ev *Event) (err error) {
	b := ev.b
	st := len(b) + 1

	b = append(b, '{')

	if ev.Span != (ID{}) {
		i := len(b)
		b = append(b, `"s":"123456789_123456789_123456789_12"`...)
		ev.Span.FormatTo(b[i+5:], 'x')
	}

	if ev.Time != (time.Time{}) {
		if len(b) > st {
			b = append(b, ',')
		}

		b = append(b, `"t":`...)
		b = strconv.AppendInt(b, ev.Time.UnixNano(), 10)
	}

	if ev.Type != 0 {
		if len(b) > st {
			b = append(b, ',')
		}

		b = append(b, `"T":"`...)
		if ev.Type == '"' {
			b = append(b, '\\', '"', '"')
		} else {
			b = append(b, byte(ev.Type), '"')
		}
	}

	if lv := int(ev.Level); lv != 0 {
		if len(b) > st {
			b = append(b, ',')
		}

		i := len(b) + 4
		b = append(b, `"l":    `...)

		if lv < 0 {
			b[i] = '-'
			i++

			lv = -lv
		}

		switch {
		case lv < 10:
			b[i] = digitsx[lv]
			i++
		case lv < 100:
			b[i] = digitsx[lv/10]
			i++
			b[i] = digitsx[lv%10]
			i++
		default:
			b[i] = digitsx[lv/100]
			i++
			b[i] = digitsx[lv/10%10]
			i++
			b[i] = digitsx[lv%10]
			i++
		}

		b = b[:i]
	}

	if ev.PC != 0 {
		if len(b) > st {
			b = append(b, ',')
		}

		b = append(b, `"pc":`...)
		b = strconv.AppendUint(b, uint64(ev.PC), 10)
	}

	for _, a := range ev.Attrs {
		if len(b) > st {
			b = append(b, ',')
		}

		b = w.appendA(b, a)
	}

	b = append(b, '}', '\n')

	ev.b = b

	_, err = w.Writer.Write(b[st-1:])

	return
}

func (w *JSONWriter) appendA(b []byte, a A) []byte {
	b = append(b, '"')
	b = appendSafe(b, a.Name)
	b = append(b, '"', ':')

	switch v := a.Value.(type) {
	case *string:
		b = AppendPrintf(b, `%q`, *v)
	case *int, *int64, *int32, *int16, *int8,
		*uint, *uint64, *uint32, *uint16, *uint8,
		*float64, *float32:
		b = AppendPrintf(b, `%v`, *v)
	case *ID:
		i := len(b)
		b = append(b, `"123456789_123456789_123456789_12"`...)
		v.FormatTo(b[i+1:], 'x')
	case fmt.Stringer:
		b = AppendPrintf(b, `%q`, v)
	case *Labels:
		b = append(b, '[')
		for i, l := range *v {
			if i != 0 {
				b = append(b, ',')
			}
			b = AppendPrintf(b, `%q`, l)
		}
		b = append(b, ']')
	case *time.Time:
		b = AppendPrintf(b, `%v`, v.UnixNano())
	case *A:
		b = append(b, '{')
		b = AppendPrintf(b, `%q`, v.Value)
		b = append(b, '}')
	case *[]A:
		if v == nil {
			b = append(b, "null"...)
		} else {
			b = append(b, '{')
			for i, a := range v {
				if i != 0 {
					b = append(b, ',')
				}

				b = w.appendA(b, a)
			}
			b = append(b, '}')
		}
	case D:
		if v == nil {
			b = append(b, "null"...)
		} else {
			b = append(b, '{')
			st := len(b)
			for k, v := range v {
				if len(b) != st {
					b = append(b, ',')
				}

				b = w.appendA(b, A{Name: k, Value: v})
			}
			b = append(b, '}')
		}
	default:
		panic(v)
	}

	return b
}
