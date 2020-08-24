package tlprometheus

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/nikandfor/quantile"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"google.golang.org/protobuf/proto"

	"github.com/nikandfor/tlog"
)

type (
	Writer struct {
		tlog.DiscardWriter

		mu sync.RWMutex
		n  map[string]*desc
		m  map[uintptr]*metric // hash -> metric

		Logger *tlog.Logger
	}

	desc struct {
		Name        string
		LabelValues []string
		ConstLabels map[string]string

		Help string
		Type string

		qtargets []float64
		m        map[uintptr]*metric // hash -> metric

		p *prometheus.Desc
	}

	metric struct {
		Labels   tlog.Labels
		Last     float64
		Count    uint64
		Sum      float64
		Quantile *quantile.Stream

		d *desc
		w *Writer

		ls []*dto.LabelPair
	}
)

var _ tlog.Writer = &Writer{}

func New() *Writer {
	return &Writer{
		n: make(map[string]*desc),
		m: make(map[uintptr]*metric),
	}
}

func (w *Writer) Meta(m tlog.Meta) error {
	if m.Type != "metric_desc" {
		return nil
	}

	defer w.mu.Unlock()
	w.mu.Lock()

	d := &desc{}

	i := 0
loop:
	for ; i < len(m.Data); i++ {
		l := m.Data[i]

		switch {
		case strings.HasPrefix(l, "name="):
			d.Name = l[5:]
		case strings.HasPrefix(l, "type="):
			d.Type = l[5:]
		case strings.HasPrefix(l, "help="):
			d.Help = l[5:]
		case l == "labels":
			i++
			break loop
		}
	}

	if i < len(m.Data) {
		ls := map[string]string{}
		for ; i < len(m.Data); i++ {
			kv := strings.SplitN(m.Data[i], "=", 2)

			if len(kv) == 1 {
				ls[kv[0]] = ""
			} else {
				ls[kv[0]] = kv[1]
			}
		}

		d.ConstLabels = ls
	}

	w.initDesc(d)

	return nil
}

func (w *Writer) initDesc(d *desc) {
	switch d.Type {
	case "", tlog.MSummary:
		d.qtargets = []float64{0.1, 0.5, 0.9, 0.95, 0.99, 1}
	}

	d.p = prometheus.NewDesc(d.Name, d.Help, nil, d.ConstLabels)

	d.m = make(map[uintptr]*metric)
	w.n[d.Name] = d
}

func (w *Writer) Metric(m tlog.Metric, sid tlog.ID) error {
	defer w.mu.Unlock()
	w.mu.Lock()

	var h uintptr
	h = tlog.StrHash(m.Name, h)
	for _, l := range m.Labels {
		h = tlog.StrHash(l, h)
	}

	mt, ok := w.m[h]
	if !ok {
		d := w.n[m.Name]

		if d == nil {
			d = &desc{
				Name: m.Name,
			}

			w.initDesc(d)
		}

		mt = &metric{
			Labels: m.Labels,
			d:      d,
			w:      w,
		}

		switch d.Type {
		case "", tlog.MSummary:
			mt.Quantile = quantile.New(0.1)
		}

		d.m[h] = mt
		w.m[h] = mt

		for k, v := range d.ConstLabels {
			mt.ls = append(mt.ls, &dto.LabelPair{
				Name:  proto.String(k),
				Value: proto.String(v),
			})
		}

		for _, l := range m.Labels {
			kv := strings.SplitN(l, "=", 2)

			ll := &dto.LabelPair{
				Name: proto.String(kv[0]),
			}
			if len(kv) != 1 {
				ll.Value = proto.String(kv[1])
				// TODO: do we need else?
			}

			mt.ls = append(mt.ls, ll)
		}

		//	w.Logger.Printf("desc: %+v", d)
		//	w.Logger.Printf("metric: %+v", mt)
		//	w.Logger.Printf("mm: %+v", m)
		//	w.Logger.Printf("writer: %+v", w)
	}

	mt.Count++
	mt.Last = m.Value
	mt.Sum += m.Value
	if mt.Quantile != nil {
		mt.Quantile.Insert(m.Value)
	}

	return nil
}

func (w *Writer) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	defer w.mu.RUnlock()
	w.mu.RLock()

	for _, d := range w.n {
		fmt.Fprintf(rw, "# HELP %v %v\n", d.Name, d.Help)
		fmt.Fprintf(rw, "# TYPE %v %v\n", d.Name, d.Type)

		switch d.Type {
		case "", tlog.MSummary:
			for _, mt := range d.m {
				//	w.Logger.Printf("qq: %+v", mt.Quantile)

				for _, q := range d.qtargets {
					v := mt.Quantile.Query(q)

					fmt.Fprintf(rw, "%v%v %v\n", d.Name, DtoLabelsToString(mt.ls, q), v)
				}

				fmt.Fprintf(rw, "%v_sum%v %v\n", d.Name, DtoLabelsToString(mt.ls, -1), mt.Sum)
				fmt.Fprintf(rw, "%v_count%v %v\n", d.Name, DtoLabelsToString(mt.ls, -1), mt.Count)
			}
		default:
			w.Logger.Printf("unsupported metric type: %q", d.Type)
		}
	}
}

func LabelsToString(ls tlog.Labels) string {
	var b strings.Builder

	b.WriteByte('{')
	for i, l := range ls {
		if i != 0 {
			b.WriteByte(',')
		}

		kv := strings.SplitN(l, "=", 2)

		b.WriteString(kv[0])
		b.WriteByte('=')

		b.WriteByte('"')
		b.WriteString(kv[1])
		b.WriteByte('"')
	}
	b.WriteByte('}')

	return b.String()
}

func DtoLabelsToString(ls []*dto.LabelPair, q float64) string {
	var b strings.Builder

	b.WriteByte('{')
	for i, l := range ls {
		if i != 0 {
			b.WriteByte(',')
		}

		b.WriteString(*l.Name)
		b.WriteByte('=')

		b.WriteByte('"')
		b.WriteString(*l.Value)
		b.WriteByte('"')
	}
	if q >= 0 {
		if len(ls) != 0 {
			b.WriteByte(',')
		}

		fmt.Fprintf(&b, "quantile=\"%v\"", q)
	}
	b.WriteByte('}')

	return b.String()
}

func (w *Writer) Describe(c chan<- *prometheus.Desc) {
	w.Logger.Printf("describe called")

	defer w.mu.RUnlock()
	w.mu.RLock()

	for _, d := range w.n {
		c <- d.p
	}
}

func (w *Writer) Collect(c chan<- prometheus.Metric) {
	w.Logger.Printf("collect called")

	defer w.mu.RUnlock()
	w.mu.RLock()

	for _, m := range w.m {
		c <- m
	}
}

func (m *metric) Desc() *prometheus.Desc {
	m.w.Logger.Printf("Metric.Desc called")

	defer m.w.mu.RUnlock()
	m.w.mu.RLock()

	return m.d.p
}

func (m *metric) Write(pb *dto.Metric) error {
	m.w.Logger.Printf("Metric.Write called")

	defer m.w.mu.RUnlock()
	m.w.mu.RLock()

	pb.Label = m.ls

	switch m.d.Type {
	case "", tlog.MSummary:
		return m.writeQuantile(pb)
	}

	return errors.New("unsupported metric type")
}

func (m *metric) writeQuantile(pb *dto.Metric) error {
	sum := &dto.Summary{
		SampleCount: proto.Uint64(m.Count),
		SampleSum:   proto.Float64(m.Sum),
	}

	for _, q := range m.d.qtargets {
		v := m.Quantile.Query(q)

		sum.Quantile = append(sum.Quantile, &dto.Quantile{
			Quantile: proto.Float64(q),
			Value:    proto.Float64(v),
		})
	}

	pb.Summary = sum

	return nil
}
