package parse

import (
	"github.com/nikandfor/tlog"
)

type (
	ID = tlog.ID

	Labels struct {
		Span   ID          `json:"s"`
		Labels tlog.Labels `json:"L"`
	}

	Frame struct {
		PC    uint64 `json:"p"`
		Entry uint64 `json:"e"`
		Name  string `json:"n"`
		File  string `json:"f"`
		Line  int    `json:"l"`
	}

	SpanStart struct {
		ID     ID `json:"i"`
		Parent ID `json:"p"`

		PC uint64 `json:"l"`

		StartedAt int64 `json:"s"`
	}

	SpanFinish struct {
		ID      ID    `json:"i"`
		Elapsed int64 `json:"e"`
	}

	Message struct {
		Span ID     `json:"s"`
		PC   uint64 `json:"l"`
		Time int64  `json:"t"`
		Text string `json:"m"`
	}

	Metric struct {
		Span   ID          `json:"s"`
		Labels tlog.Labels `json:"L"`
		Name   string      `json:"n"`
		Value  float64     `json:"v"`
		Hash   int64       `json:"h"`
	}

	Meta struct {
		Type string   `json:"type"`
		Data []string `json:"data"`
	}

	Type rune

	Reader interface {
		Read() (interface{}, error)
	}

	LowReader interface {
		Type() (Type, error)
		Read() (interface{}, error)

		Labels() (Labels, error)
		Frame() (Frame, error)
		Message() (Message, error)
		Metric() (Metric, error)
		SpanStart() (SpanStart, error)
		SpanFinish() (SpanFinish, error)
	}
)

const (
	TypeNone       Type = 0
	TypeLabels     Type = 'L'
	TypeFrame      Type = 'l'
	TypeMessage    Type = 'm'
	TypeMetric     Type = 'v'
	TypeSpanStart  Type = 's'
	TypeSpanFinish Type = 'f'
)

func (t Type) String() string {
	if t == 0 {
		return `0`
	}
	return string(t)
}
