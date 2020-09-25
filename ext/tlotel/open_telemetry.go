package tlotel

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/api/global"
	"go.opentelemetry.io/otel/api/metric"
	"go.opentelemetry.io/otel/api/trace"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/label"

	"github.com/nikandfor/tlog"
)

type (
	Provider struct {
		Getter func(n string) *tlog.Logger
	}

	Tracer struct {
		Logger *tlog.Logger
	}

	Meter struct {
		Logger *tlog.Logger
	}

	Sync struct {
		Logger *tlog.Logger
		Desc   metric.Descriptor

		name string
	}

	Span struct {
		tlog.Span
	}
)

func InitByDefault() {
	InitCustom(func(string) *tlog.Logger { return tlog.DefaultLogger })
}

func InitLogger(l *tlog.Logger) {
	InitCustom(func(string) *tlog.Logger { return l })
}

func InitCustom(f func(n string) *tlog.Logger) {
	p := &Provider{
		Getter: f,
	}

	global.SetTraceProvider(p)

	global.SetMeterProvider(p)
}

func (p *Provider) Tracer(n string, opts ...trace.TracerOption) trace.Tracer {
	return Tracer{Logger: p.Getter(n)}
}

func (p *Provider) Meter(n string, opts ...metric.MeterOption) metric.Meter {
	return metric.WrapMeterImpl(Meter{Logger: p.Getter(n)}, "tlog", opts...)
}

func (m Meter) RecordBatch(ctx context.Context, kv []label.KeyValue, ms ...metric.Measurement) {
}

func (m Meter) NewSyncInstrument(d metric.Descriptor) (metric.SyncImpl, error) { //nolint:gocritic
	return &Sync{Logger: m.Logger, Desc: d, name: d.Name()}, nil
}

func (m Meter) NewAsyncInstrument(d metric.Descriptor, r metric.AsyncRunner) (metric.AsyncImpl, error) { //nolint:gocritic
	panic("wtf it is?")
}

func (m *Sync) Implementation() interface{} { panic("wtf it is?") }

func (m *Sync) Descriptor() metric.Descriptor { return m.Desc }

func (m *Sync) Bind(ls []label.KeyValue) metric.BoundSyncImpl {
	panic("wtf it is?")
}

func (m *Sync) RecordOne(ctx context.Context, n metric.Number, kvs []label.KeyValue) {
	var v float64

	switch m.Desc.NumberKind() {
	case metric.Int64NumberKind:
		v = float64(n.AsInt64())
	case metric.Float64NumberKind:
		v = n.AsFloat64()
	default:
		panic("not implemented")
	}

	ls := labels(nil, kvs)

	s := tlog.SpanFromContext(ctx)
	if s.Valid() {
		s.Observe(m.name, v, ls)
	} else {
		m.Logger.Observe(m.name, v, ls)
	}
}

func (t Tracer) Start(ctx context.Context, spanName string, opts ...trace.StartOption) (context.Context, trace.Span) {
	if !t.Logger.Valid() {
		return ctx, Span{}
	}

	var cfg trace.StartConfig

	for _, o := range opts {
		o(&cfg)
	}

	if !cfg.Record {
		return ctx, Span{}
	}

	s := tlog.Span{
		ID: t.Logger.RandID(),
	}

	se := tlog.SpanStart{
		ID: s.ID,
	}

	if !cfg.NewRoot && len(cfg.Links) != 0 {
		se.Parent = tlog.ID(cfg.Links[0].SpanContext.TraceID)
	}

	if cfg.StartTime != (time.Time{}) {
		s.Started = cfg.StartTime
	} else {
		s.Started = time.Now()
	}

	se.Started = s.Started.UnixNano()

	if !t.Logger.NoCaller {
		se.PC = tlog.Funcentry(1)
	}

	_ = t.Logger.SpanStarted(se)

	var as tlog.Attrs

	if cfg.Attributes != nil {
		as = attrs(as[:0], cfg.Attributes)

		s.PrintwDepth(1, "attrs", as...)
	}

	for i, l := range cfg.Links {
		if i == 0 && len(l.Attributes) == 0 {
			continue
		}

		as = append(as[:0], tlog.Attr{Name: "_span", Value: tlog.ID(l.SpanContext.TraceID)})

		as = attrs(as, l.Attributes)

		s.PrintwDepth(1, "link", as...)
	}

	if cfg.SpanKind != 0 {
		s.PrintwDepth(1, "span_kind", tlog.AInt("span_kind", int(cfg.SpanKind)))
	}

	ctx = tlog.ContextWithSpan(ctx, s)

	return ctx, Span{s}
}

func (s Span) Tracer() trace.Tracer {
	return Tracer{Logger: s.Logger}
}

func (s Span) End(opts ...trace.EndOption) {
	if !s.Span.Valid() {
		return
	}

	var cfg trace.EndConfig

	for _, o := range opts {
		o(&cfg)
	}

	e := tlog.SpanFinish{
		ID: s.Span.ID,
	}

	if cfg.EndTime != (time.Time{}) {
		e.Elapsed = cfg.EndTime.Sub(s.Span.Started).Nanoseconds()
	} else {
		e.Elapsed = time.Since(s.Span.Started).Nanoseconds()
	}

	_ = s.Span.Logger.SpanFinished(e)
}

func (s Span) AddEventWithTimestamp(ctx context.Context, tm time.Time, name string, kvs ...label.KeyValue) {
	if !s.Span.Valid() {
		return
	}

	m := tlog.Message{
		Text: name,
	}

	if !s.Span.Logger.NoCaller {
		m.PC = tlog.Caller(1)
	}
	if tm != (time.Time{}) {
		m.Time = tm.UnixNano()
	}

	m.Attrs = attrs(nil, kvs)

	_ = s.Span.Logger.Message(m, s.Span.ID)
}

func (s Span) AddEvent(ctx context.Context, name string, attrs ...label.KeyValue) {
	s.AddEventWithTimestamp(ctx, time.Time{}, name, attrs...)
}

func (s Span) IsRecording() bool { return s.Span.Valid() }

func (s Span) RecordError(ctx context.Context, err error, opts ...trace.ErrorOption) {
	var cfg trace.ErrorConfig

	for _, o := range opts {
		o(&cfg)
	}

	s.Span.SetError()

	m := tlog.Message{
		Text:  "error",
		PC:    tlog.Caller(1),
		Attrs: tlog.Attrs{{Name: "error", Value: err}},
	}

	if cfg.Timestamp != (time.Time{}) {
		m.Time = cfg.Timestamp.UnixNano()
	} else {
		m.Time = time.Now().UnixNano()
	}

	if cfg.StatusCode != 0 {
		m.Attrs = append(m.Attrs, tlog.AInt("code", int(cfg.StatusCode)))
	}

	_ = s.Span.Logger.Message(m, s.Span.ID)
}

func (s Span) SpanContext() (r trace.SpanContext) {
	r.TraceID = trace.ID(s.Span.ID)
	// r.SpanID is 8 bytes long

	return
}

func (s Span) SetStatus(c codes.Code, msg string) {
	s.Span.PrintwDepth(1, "status", tlog.Attrs{
		{Name: "code", Value: c},
		{Name: "message", Value: msg},
	}...)
}

func (s Span) SetName(n string) {
	s.Span.PrintwDepth(1, "name", tlog.AString("name", n))
}

func (s Span) SetAttributes(kv ...label.KeyValue) {
	s.Span.PrintwDepth(1, "attributes", attrs(nil, kv)...)
}

func (s Span) SetAttribute(k string, v interface{}) {
	s.Span.PrintwDepth(1, "attributes", tlog.Attr{Name: k, Value: v})
}

func attrs(b tlog.Attrs, as []label.KeyValue) tlog.Attrs {
	if len(as) == 0 {
		return b
	}

	for _, a := range as {
		b = append(b, tlog.Attr{
			Name:  string(a.Key),
			Value: a.Value.AsInterface(),
		})
	}

	return b
}

func labels(b tlog.Labels, ls []label.KeyValue) tlog.Labels {
	if len(ls) == 0 {
		return b
	}

	for _, a := range ls {
		l := string(a.Key)

		if v := a.Value.Emit(); v != "" {
			l += "=" + v
		}

		b = append(b, l)
	}

	return b
}
