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
)

type (
	Writer struct {
		d tlog.Decoder

		mu sync.Mutex

		ls     tlog.Labels
		labels []byte

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
	w.d.ResetBytes(p)

	e := w.getEventType()

	switch e {
	case tlog.EventValue:
		w.value()
	case tlog.EventLabels:
		w.ls = w.getLabels()
		w.labels = w.encodeLabels(w.labels[:0], w.ls)

	//	tlog.Printw("labels", "global", w.labels)
	case tlog.EventMetricDesc:
		w.metric()
	}

	return len(p), nil
}

func (w *Writer) value() {
	var i int64
	_, els, i := w.d.Tag(i)

	var m *metric
	var val interface{}
	var ls []byte

	var k, s []byte
	var v interface{}
	for el := 0; els == -1 || el < els; el++ {
		if els == -1 && w.d.Break(&i) {
			break
		}

		k, i = w.d.String(i)

		tag, sub, _ := w.d.Tag(i)
		if tag == tlog.Semantic {
			i = w.d.Skip(i)

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
			case tlog.Int, tlog.Neg:
				val, i = w.d.Int(i)
			case tlog.Special:
				switch sub {
				case tlog.Float64, tlog.Float32, tlog.Float16, tlog.FloatInt8:
					val, i = w.d.Float(i)
				default:
					i = w.d.Skip(i)
				}
			default:
				i = w.d.Skip(i)
			}

			continue
		}

		switch tag {
		case tlog.String:
			s, i = w.d.String(i)
		case tlog.Int, tlog.Neg:
			v, i = w.d.Int(i)
		case tlog.Special:
			switch sub {
			case tlog.Float64, tlog.Float32, tlog.Float16, tlog.FloatInt8:
				v, i = w.d.Float(i)
			default:
				i = w.d.Skip(i)
			}
		default:
			i = w.d.Skip(i)

			continue
		}

		ls = append(ls, k...)
		ls = append(ls, '=', '"')

		if s != nil {
			ls = low.AppendSafe(ls, low.UnsafeBytesToString(s))
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

func (w *Writer) metric() {
	var i int64
	_, els, i := w.d.Tag(i)

	m := &metric{}

	var k, s []byte
	for el := 0; els == -1 || el < els; el++ {
		if els == -1 && w.d.Break(&i) {
			break
		}

		k, i = w.d.String(i)

		tag, sub, _ := w.d.Tag(i)

		switch {
		case tag == tlog.String && string(k) == "name":
			s, i = w.d.String(i)

			m.Name = string(s)
		case tag == tlog.String && string(k) == "type":
			s, i = w.d.String(i)

			m.Type = string(s)
		case tag == tlog.String && string(k) == "help":
			s, i = w.d.String(i)

			m.Help = string(s)
		case tag == tlog.Semantic && sub == tlog.WireLabels && string(k) == tlog.KeyLabels:
			var ls tlog.Labels
			ls, i = w.d.Labels(i)

			m.constls = w.encodeLabels(nil, ls)
		case tag == tlog.Array && string(k) == "quantile":
			st := i

			_, _, i = w.d.Tag(i)

			for el := 0; sub == -1 || el < sub; el++ {
				if sub == -1 && w.d.Break(&i) {
					break
				}

				var f float64
				f, i = w.d.Float(i)
				if w.d.Err() != nil {
					w.d.ResetErr()
					break
				}

				m.q = append(m.q, f)
			}

			i = w.d.Skip(st)
		default:
			i = w.d.Skip(i)
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

func (w *Writer) getEventType() (e tlog.EventType) {
	i := w.find(tlog.WireEventType, tlog.KeyEventType)
	if i == -1 {
		return
	}

	e, _ = w.d.EventType(i)

	return
}

func (w *Writer) getLabels() (ls tlog.Labels) {
	i := w.find(tlog.WireLabels, tlog.KeyLabels)
	if i == -1 {
		return
	}

	ls, _ = w.d.Labels(i)

	return ls
}

func (w *Writer) find(typ int, key string) (i int64) {
	tag, els, i := w.d.Tag(i)

	if tag != tlog.Map {
		return -1
	}

	var k []byte
	for el := 0; els == -1 || el < els; el++ {
		if els == -1 && w.d.Break(&i) {
			break
		}

		k, i = w.d.String(i)

		tag, sub, _ := w.d.Tag(i)
		if tag == tlog.Semantic && sub == typ && string(k) == key {
			return i
		}

		i = w.d.Skip(i)
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

				commals := append(ls, ',')

				for _, q := range m.q {
					v := o.q.Query(q)

					//	tlog.Printw("metric", "name", m.Name, "global", w.labels, "const", m.constls, "val", m.ls[sum])

					fmt.Fprintf(rw, "%v{%squantile=\"%v\"} %v\n", m.Name, commals, q, v)
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

	if len(w.labels) != 0 {
		comma = true

		ls = append(ls, w.labels...)
	}

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
