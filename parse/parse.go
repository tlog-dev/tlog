package parse

import (
	"time"

	"github.com/nikandfor/tlog"
)

type (
	ID     = tlog.ID
	Labels = tlog.Labels

	Location struct {
		PC   uintptr `json:"p"`
		Name string  `json:"n"`
		File string  `json:"f"`
		Line int     `json:"l"`
	}

	SpanStart struct {
		ID     ID
		Parent ID

		Location uintptr

		Started time.Time
	}

	SpanFinish struct {
		ID      ID
		Elapsed time.Duration
	}

	Message struct {
		Span     ID
		Location uintptr
		Time     time.Duration
		Text     string
	}

	Type rune

	Reader interface {
		Read() (interface{}, error)
	}

	LowReader interface {
		Type() (Type, error)
		Read() (interface{}, error)

		Labels() (Labels, error)
		Location() (Location, error)
		Message() (Message, error)
		SpanStart() (SpanStart, error)
		SpanFinish() (SpanFinish, error)
	}
)

const (
	TypeNone       Type = 0
	TypeLabels     Type = 'L'
	TypeLocation   Type = 'l'
	TypeMessage    Type = 'm'
	TypeSpanStart  Type = 's'
	TypeSpanFinish Type = 'f'
)

func (t Type) String() string {
	return string(t)
}
