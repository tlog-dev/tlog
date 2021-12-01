package tlprometheus

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
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

		metrics []*metric
		byname  map[string]*metric
	}

	metric struct {
		Name string
		Type string
		Help string

		ls string

		vals map[string]observer // labels -> observer
	}

	observer interface {
		Observe(v float64)
	}

	gauge struct {
		v float64
	}

	counter struct {
		v float64
	}

	summary struct {
		v *quantile.Stream

		sum   float64
		count int64

		quantiles []float64
	}
)

func New() *Writer {
	return &Writer{
		byname: make(map[string]*metric),
	}
}

func (w *Writer) Write(p []byte) (_ int, err error) {
	ev := w.getEventKind(p)

	switch ev {
	case tlog.EventValue:
		err = w.value(p)
	case tlog.EventMetricDesc:
		err = w.metric(p)
	}
	if err != nil {
		return 0, err
	}

	return len(p), nil
}

func (w *Writer) value(p []byte) error {
	tag, els, i := w.d.Tag(p, 0)
	if tag != wire.Map {
		return errors.New("map expected")
	}

	var name []byte
	var val float64
	var ls []byte

	for el := 0; els == -1 || el < int(els); el++ {
		if els == -1 && w.d.Break(p, &i) {
			break
		}

		var k []byte
		k, i = w.d.String(p, i)

		tag, sub, _ := w.d.Tag(p, i)

		if tag == wire.Semantic {
			switch {
			case sub == wire.Time && string(k) == tlog.KeyTime,
				sub == tlog.WireID && string(k) == tlog.KeySpan,
				sub == tlog.WireID && string(k) == tlog.KeyParent,
				sub == tlog.WireEventKind && string(k) == tlog.KeyEventKind,
				false:

				i = w.d.Skip(p, i)
				continue
			}
		}

		if name == nil {
			name = k

			val, i = w.val(p, i)

			continue
		}

		switch tag {
		case wire.Int, wire.Neg, wire.String:
		default:
			i = w.d.Skip(p, i)
			continue
		}

		if len(ls) != 0 {
			ls = append(ls, ',')
		}

		ls = append(ls, k...)
		ls = append(ls, '=')

		switch tag {
		case wire.Int, wire.Neg:
			if tag == wire.Neg {
				ls = append(ls, '-')
			}

			var x uint64
			x, i = w.d.Unsigned(p, i)

			ls = strconv.AppendUint(ls, x, 10)
		case wire.String:
			k, i = w.d.String(p, i)

			ls = low.AppendQuote(ls, k)
		default:
			panic(tag)
		}

		_ = sub
	}

	if name == nil {
		return nil
	}

	defer w.mu.Unlock()
	w.mu.Lock()

	m, ok := w.byname[string(name)]
	if !ok {
		m = &metric{
			Name: string(name),
			Type: "gauge",
			Help: "unexpected metric",
			vals: make(map[string]observer),
		}

		w.byname[string(name)] = m

		w.metrics = append(w.metrics, m)

		sort.Slice(w.metrics, w.byNameSorter)
	}

	o, ok := m.vals[string(ls)]
	if !ok {
		o = w.newObserver(m.Type)
		m.vals[string(ls)] = o
	}

	o.Observe(val)

	return nil
}

func (w *Writer) val(p []byte, st int) (val float64, i int) {
	tag, sub, i := w.d.Tag(p, st)

	switch tag {
	case wire.Int, wire.Neg:
		val = float64(sub)

		return
	case wire.Special:
		switch sub {
		case wire.Float64, wire.Float32, wire.Float16, wire.Float8:
			return w.d.Float(p, st)
		}
	case wire.Semantic:
		return w.val(p, i)
	}

	panic(fmt.Sprintf("val %v %x  st %x\n%v", wire.Tag(tag), sub, st, wire.Dump(p)))
}

func (w *Writer) metric(p []byte) error {
	tag, els, i := w.d.Tag(p, 0)
	if tag != wire.Map {
		return errors.New("map expected")
	}

	var name, typ, help []byte

	for el := 0; els == -1 || el < int(els); el++ {
		if els == -1 && w.d.Break(p, &i) {
			break
		}

		var k []byte

		k, i = w.d.String(p, i)

		switch string(k) {
		case "name":
			name, i = w.d.String(p, i)
		case "type":
			typ, i = w.d.String(p, i)
		case "help":
			help, i = w.d.String(p, i)
		default:
			i = w.d.Skip(p, i)
		}
	}

	if name == nil || typ == nil {
		return nil
	}

	defer w.mu.Unlock()
	w.mu.Lock()

	m, ok := w.byname[string(name)]
	if !ok {
		m = &metric{
			Name: string(name),
			vals: make(map[string]observer),
		}

		w.byname[string(name)] = m

		w.metrics = append(w.metrics, m)

		sort.Slice(w.metrics, w.byNameSorter)
	}

	m.Type = string(typ)
	if help != nil {
		m.Help = string(help)
	}

	return nil
}

func (w *Writer) newObserver(tp string) observer {
	switch tp {
	case tlog.MetricGauge:
		return &gauge{}
	case tlog.MetricCounter:
		return &counter{}
	case tlog.MetricSummary:
		return &summary{
			v: quantile.New(0.1),

			quantiles: []float64{0.1, 0.5, 0.9, 0.95, 0.99},
		}
	default:
		return &gauge{}
	}
}

func (w *Writer) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	var b []byte

	labels := func(ls0, ls string, q float64) {
		if ls0 == "" && ls == "" && q < 0 {
			b = append(b, ' ')
			return
		}

		b = append(b, '{')

		if ls0 != "" {
			b = append(b, ls0...)
		}

		if ls0 != "" && ls != "" {
			b = append(b, ',')
		}

		if ls != "" {
			b = append(b, ls...)
		}

		if ls != "" && q >= 0 {
			b = append(b, ',')
		}

		if q >= 0 {
			b = append(b, "quantile=\""...)

			b = strconv.AppendFloat(b, q, 'f', -1, 32)

			b = append(b, '"')
		}

		b = append(b, '}', ' ')
	}

	value := func(v float64) {
		b = strconv.AppendFloat(b, v, 'f', -1, 64)
		b = append(b, '\n')

		_, _ = rw.Write(b)
	}

	count := func(v int64) {
		b = strconv.AppendUint(b, uint64(v), 10)
		b = append(b, '\n')

		_, _ = rw.Write(b)
	}

	line := func(n, ls0, ls string, v float64) {
		b = append(b[:0], n...)
		labels(ls0, ls, -1)
		value(v)
	}

	summline := func(n, ls0, ls string, q, v float64) {
		b = append(b[:0], n...)
		labels(ls0, ls, q)
		value(v)
	}

	defer w.mu.Unlock()
	w.mu.Lock()

	for _, m := range w.metrics {
		if len(m.vals) == 0 {
			continue
		}

		fmt.Fprintf(rw, "# HELP %v %v\n", m.Name, m.Help)
		fmt.Fprintf(rw, "# TYPE %v %v\n", m.Name, m.Type)

		for ls, o := range m.vals {
			switch v := o.(type) {
			case *gauge:
				line(m.Name, m.ls, ls, v.v)
			case *counter:
				line(m.Name, m.ls, ls, v.v)
			case *summary:
				for _, q := range v.quantiles {
					summline(m.Name, m.ls, ls, q, v.v.Query(q))
				}

				b = append(b[:0], m.Name...)
				b = append(b, "_sum"...)
				labels(m.ls, ls, -1)
				value(v.sum)

				b = append(b[:0], m.Name...)
				b = append(b, "_count"...)
				labels(m.ls, ls, -1)
				count(v.count)
			default:
				fmt.Fprintf(rw, "# error: unsupported type: %T\n", v)
			}
		}
	}
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

func (w *Writer) byNameSorter(i, j int) bool {
	return w.metrics[i].Name < w.metrics[j].Name
}

func (v *gauge) Observe(val float64) {
	v.v = val
}

func (v *counter) Observe(val float64) {
	v.v += val
}

func (v *summary) Observe(val float64) {
	v.v.Insert(val)
	v.sum += val
	v.count++
}
