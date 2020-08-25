package tlprometheus

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

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
		s  map[tlog.ID]span    // span id -> started

		labels tlog.Labels
		dtols  []*dto.LabelPair

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

	span struct {
		Location tlog.Location
		Labels   tlog.Labels
	}
)

var _ tlog.Writer = &Writer{}

func New() *Writer {
	return &Writer{
		n: make(map[string]*desc),
		m: make(map[uintptr]*metric),
		s: make(map[tlog.ID]span),
	}
}

func (w *Writer) Labels(ls tlog.Labels, sid tlog.ID) error {
	defer w.mu.Unlock()
	w.mu.Lock()

	if sid == (tlog.ID{}) {
		w.labels = append(w.labels[:0], ls...)

		w.dtols = w.dtols[:0]

		for _, l := range ls {
			kv := strings.SplitN(l, "=", 2)

			ll := &dto.LabelPair{
				Name: proto.String(kv[0]),
			}
			if len(kv) != 1 {
				ll.Value = proto.String(kv[1])
				// TODO: do we need else?
			}

			w.dtols = append(w.dtols, ll)
		}

		return nil
	}

	sp := w.s[sid]
	sp.Labels = ls
	w.s[sid] = sp

	return nil
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

func (w *Writer) metric(n string, sid tlog.ID, ls tlog.Labels) *metric {
	d := w.n[n]

	var sp span
	if sid != (tlog.ID{}) {
		sp = w.s[sid]
	}

	var h uintptr
	h = tlog.StrHash(n, h)
	for _, l := range w.labels {
		h = tlog.StrHash(l, h)
	}
	for _, l := range sp.Labels {
		h = tlog.StrHash(l, h)
	}
	for _, l := range d.ConstLabels {
		h = tlog.StrHash(l, h)
	}
	for _, l := range ls {
		h = tlog.StrHash(l, h)
	}

	mt, ok := w.m[h]
	if ok {
		return mt
	}

	dtols := make([]*dto.LabelPair, len(w.dtols), len(w.dtols)+len(sp.Labels)+len(ls))

	copy(dtols, w.dtols)

	for _, l := range sp.Labels {
		p := strings.IndexRune(l, '=')

		if p == -1 {
			dtols = append(dtols, &dto.LabelPair{
				Name: proto.String(l),
			})
		} else {
			dtols = append(dtols, &dto.LabelPair{
				Name:  proto.String(l[:p]),
				Value: proto.String(l[p+1:]),
			})
		}
	}

	for k, v := range d.ConstLabels {
		if v == "" {
			dtols = append(dtols, &dto.LabelPair{
				Name: proto.String(k),
			})
		} else {
			dtols = append(dtols, &dto.LabelPair{
				Name:  proto.String(k),
				Value: proto.String(v),
			})
		}
	}

	for _, l := range ls {
		p := strings.IndexRune(l, '=')

		if p == -1 {
			dtols = append(dtols, &dto.LabelPair{
				Name: proto.String(l),
			})
		} else {
			dtols = append(dtols, &dto.LabelPair{
				Name:  proto.String(l[:p]),
				Value: proto.String(l[p+1:]),
			})
		}
	}

	mt = &metric{
		Labels:   ls,
		Quantile: quantile.New(0.1),
		d:        d,
		w:        w,
		ls:       dtols,
	}

	d.m[h] = mt
	w.m[h] = mt

	return mt
}

func (w *Writer) Metric(m tlog.Metric, sid tlog.ID) error {
	defer w.mu.Unlock()
	w.mu.Lock()

	mt := w.metric(m.Name, sid, m.Labels)

	mt.Count++
	mt.Last = m.Value
	mt.Sum += m.Value
	if mt.Quantile != nil {
		mt.Quantile.Insert(m.Value)
	}

	return nil
}

func (w *Writer) SpanStarted(id, par tlog.ID, st int64, l tlog.Location) error {
	defer w.mu.Unlock()
	w.mu.Lock()

	w.s[id] = span{Location: l}

	return nil
}

func (w *Writer) SpanFinished(id tlog.ID, el int64) error {
	defer w.mu.Unlock()
	w.mu.Lock()

	sp := w.s[id]
	defer delete(w.s, id)

	if sp.Location == 0 {
		return nil
	}

	dur := float64(el) / float64(time.Millisecond)

	name, _, _ := sp.Location.NameFileLine()
	//	name = path.Base(name)

	ls := tlog.Labels{"func=" + name}

	_, ok := w.n["span_duration_ms"]
	if !ok {
		d := &desc{
			Name: "span_duration_ms",
			Type: tlog.MSummary,
			Help: "span context duration in milliseconds",
		}

		w.initDesc(d)
	}

	mt := w.metric("span_duration_ms", id, ls)

	mt.Count++
	mt.Last = dur
	mt.Sum += dur
	mt.Quantile.Insert(dur)

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
	defer w.mu.RUnlock()
	w.mu.RLock()

	for _, d := range w.n {
		c <- d.p
	}
}

func (w *Writer) Collect(c chan<- prometheus.Metric) {
	defer w.mu.RUnlock()
	w.mu.RLock()

	for _, m := range w.m {
		c <- m
	}
}

func (m *metric) Desc() *prometheus.Desc {
	defer m.w.mu.RUnlock()
	m.w.mu.RLock()

	return m.d.p
}

func (m *metric) Write(pb *dto.Metric) error {
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
