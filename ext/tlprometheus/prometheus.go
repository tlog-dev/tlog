package tlprometheus

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/nikandfor/quantile"
	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/low"
	"github.com/nikandfor/tlog/wire"
)

type (
	Writer struct {
		d wire.Decoder

		mu sync.Mutex

		vs map[string]*metric

		lsbuf []byte
	}

	observer interface {
		Observe(v interface{})
	}

	metric struct {
		Name string
		Type string
		Help string

		constls []byte

		q []float64

		ls map[uintptr][]byte
		o  map[uintptr]observer
	}

	summary struct {
		q     *quantile.Stream
		sum   float64
		count int
	}

	gauge struct {
		v interface{}
	}

	counter struct {
		v float64
	}
)

func New() *Writer {
	return &Writer{
		vs: make(map[string]*metric),
	}
}

func (w *Writer) Write(p []byte) (n int, err error) {
	e := w.getEventKind(p)

	switch e {
	case tlog.EventValue:
		w.value(p)
	case tlog.EventMetricDesc:
		w.metric(p)
	}

	return len(p), nil
}

func (w *Writer) value(p []byte) {
	_, els, i := w.d.Tag(p, 0)

	var m *metric
	var val interface{}
	var ls []byte

	var k, s []byte
	var v interface{}
	for el := 0; els == -1 || el < int(els); el++ {
		if els == -1 && w.d.Break(p, &i) {
			break
		}

		k, i = w.d.String(p, i)

		tag, sub, _ := w.d.Tag(p, i)
		if tag == wire.Semantic {
			i = w.d.Skip(p, i)

			continue
		}

		if m == nil {
			var ok bool
			m, ok = w.vs[string(k)]

			if !ok {
				m = w.initMetric(&metric{
					Name: string(k),
				})
			}

			switch tag {
			case wire.Int:
				val, i = w.d.Unsigned(p, i)
			case wire.Neg:
				val, i = w.d.Signed(p, i)
			case wire.Special:
				switch sub {
				case wire.Float64, wire.Float32, wire.Float16, wire.Float8:
					val, i = w.d.Float(p, i)
				default:
					i = w.d.Skip(p, i)
				}
			default:
				i = w.d.Skip(p, i)
			}

			continue
		}

		switch tag {
		case wire.String:
			s, i = w.d.String(p, i)
		case wire.Int:
			v, i = w.d.Unsigned(p, i)
		case wire.Neg:
			v, i = w.d.Signed(p, i)
		case wire.Special:
			switch sub {
			case wire.Float64, wire.Float32, wire.Float16, wire.Float8:
				v, i = w.d.Float(p, i)
			default:
				i = w.d.Skip(p, i)
			}
		default:
			i = w.d.Skip(p, i)

			continue
		}

		if len(ls) != 0 {
			ls = append(ls, ',')
		}

		ls = append(ls, k...)
		ls = append(ls, '=', '"')

		if s != nil {
			ls = low.AppendSafe(ls, s)
		} else {
			switch v := v.(type) {
			case int64:
				ls = strconv.AppendInt(ls, v, 10)
			case float64:
				ls = strconv.AppendFloat(ls, v, 'f', -1, 64)
			default:
				panic(v)
			}
		}

		ls = append(ls, '"')
	}

	//	tlog.Printw("value", "val", val, "ls", ls)

	m.observe(val, ls)
}

func (w *Writer) metric(p []byte) {
	_, els, i := w.d.Tag(p, 0)

	m := &metric{}

	var k, s []byte
	for el := 0; els == -1 || el < int(els); el++ {
		if els == -1 && w.d.Break(p, &i) {
			break
		}

		k, i = w.d.String(p, i)

		tag, sub, _ := w.d.Tag(p, i)

		switch {
		case tag == wire.String && string(k) == "name":
			s, i = w.d.String(p, i)

			m.Name = string(s)
		case tag == wire.String && string(k) == "type":
			s, i = w.d.String(p, i)

			m.Type = string(s)
		case tag == wire.String && string(k) == "help":
			s, i = w.d.String(p, i)

			m.Help = string(s)
		case tag == wire.Semantic && sub == tlog.WireLabels && string(k) == "labels":
			var ls tlog.Labels
			i = ls.TlogParse(&w.d, p, i)

			m.constls = w.encodeLabels(m.constls, ls)
		case tag == wire.Array && string(k) == "quantiles":
			st := i

			_, _, i = w.d.Tag(p, i)

			for el := 0; sub == -1 || el < int(sub); el++ {
				if sub == -1 && w.d.Break(p, &i) {
					break
				}

				var f float64
				f, i = w.d.Float(p, i)

				m.q = append(m.q, f)
			}

			i = w.d.Skip(p, st)
		default:
			i = w.d.Skip(p, i)
		}
	}

	if m.q == nil {
		m.q = []float64{0, 0.1, 0.5, 0.9, 0.99, 1}
	}

	w.initMetric(m)

	//	tlog.Printw("desc", "name", m.Name, "global", w.labels, "const", m.constls, "q", m.q)
}

func (w *Writer) initMetric(m *metric) *metric {
	m.ls = make(map[uintptr][]byte)
	m.o = make(map[uintptr]observer)

	w.vs[m.Name] = m

	return m
}

func (w *Writer) encodeLabels(b []byte, ls tlog.Labels) []byte {
	for _, l := range ls {
		if len(b) != 0 {
			b = append(b, ',')
		}

		kv := strings.SplitN(l, "=", 2)

		b = append(b, kv[0]...)
		b = append(b, '=', '"')
		b = append(b, kv[1]...)
		b = append(b, '"')
	}

	return b
}

func (m *metric) observe(v interface{}, ls []byte) {
	if m == nil {
		return
	}

	sum := low.BytesHash(ls, 0)

	_, ok := m.ls[sum]
	if !ok {
		m.ls[sum] = ls
	}

	o, ok := m.o[sum]
	if !ok {
		switch m.Type {
		case "", tlog.MetricSummary:
			o = newSummary()
		case tlog.MetricGauge:
			o = &gauge{}
		case tlog.MetricCounter:
			o = &counter{}
		default:
			return
		}

		m.o[sum] = o
	}

	o.Observe(v)
}

func (w *Writer) getEventKind(p []byte) (e tlog.EventKind) {
	i := w.find(p, tlog.WireEventKind, tlog.KeyEventKind)
	if i == -1 {
		return
	}

	e.TlogParse(&w.d, p, i)

	return
}

func (w *Writer) find(p []byte, typ int64, key string) (i int) {
	tag, els, i := w.d.Tag(p, i)

	if tag != wire.Map {
		return -1
	}

	var k []byte
	for el := 0; els == -1 || el < int(els); el++ {
		if els == -1 && w.d.Break(p, &i) {
			break
		}

		k, i = w.d.String(p, i)

		tag, sub, _ := w.d.Tag(p, i)
		if tag == wire.Semantic && sub == typ && string(k) == key {
			return i
		}

		i = w.d.Skip(p, i)
	}

	return -1
}

func (w *Writer) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	defer w.mu.Unlock()
	w.mu.Lock()

	for _, m := range w.vs {
		fmt.Fprintf(rw, "# HELP %v %v\n", m.Name, m.Help)
		fmt.Fprintf(rw, "# TYPE %v %v\n", m.Name, m.Type)

		for sum, o := range m.o {
			switch o := o.(type) {
			case *summary:
				ls := w.catLabels(m, sum)

				for _, q := range m.q {
					v := o.q.Query(q)

					//	tlog.Printw("metric", "name", m.Name, "global", w.labels, "const", m.constls, "val", m.ls[sum])

					fmt.Fprintf(rw, "%v{%s%squantile=\"%v\"} %v\n", m.Name, ls, ",", q, v)
				}

				fmt.Fprintf(rw, "%v_sum{%s} %v\n", m.Name, ls, o.sum)
				fmt.Fprintf(rw, "%v_count{%s} %v\n", m.Name, ls, o.count)
			case *counter:
				fmt.Fprintf(rw, "%v %v\n", m.Name, o.v)
			case *gauge:
				fmt.Fprintf(rw, "%v %v\n", m.Name, o.v)
			default:
				fmt.Fprintf(rw, "# error: unsupported type: %T\n", o)
			}
		}
	}
}

func (w *Writer) catLabels(m *metric, sum uintptr) (ls []byte) {
	var comma bool
	ls = w.lsbuf[:0]

	tlog.Printw("metric", "m", m, "sum", sum, "constls", m.constls, "m.ls", m.ls[sum])

	if len(m.constls) != 0 {
		if comma {
			ls = append(ls, ',')
		}

		comma = true

		ls = append(ls, m.constls...)
	}

	if q := m.ls[sum]; len(q) != 0 {
		if comma {
			ls = append(ls, ',')
		}

		ls = append(ls, q...)
	}

	if comma {
		w.lsbuf = append(ls, ',')

		ls = ls[:len(w.lsbuf)-1]
	}

	return
}

func newSummary() *summary {
	return &summary{
		q: quantile.New(0.01),
	}
}

func (o *summary) Observe(v interface{}) {
	switch v := v.(type) {
	case float64:
		o.q.Insert(v)
		o.sum += v
	case float32:
		o.q.Insert(float64(v))
		o.sum += float64(v)
	case int64:
		o.q.Insert(float64(v))
		o.sum += float64(v)
	default:
		panic("unsupported type")
	}

	o.count++
}

func (o *gauge) Observe(v interface{}) {
	o.v = v
}

func (o *counter) Observe(v interface{}) {
	switch v := v.(type) {
	case float64:
		o.v += v
	case float32:
		o.v += float64(v)
	case int64:
		o.v += float64(v)
	default:
		panic("unsupported type")
	}
}
