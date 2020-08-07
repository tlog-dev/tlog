package parse

import (
	"time"

	"github.com/nikandfor/tlog"
)

type (
	ID = tlog.ID

	Labels struct {
		Span   ID          `json:"s"`
		Labels tlog.Labels `json:"L"`
	}

	Location struct {
		PC    uintptr `json:"p"`
		Entry uintptr `json:"e"`
		Name  string  `json:"n"`
		File  string  `json:"f"`
		Line  int     `json:"l"`
	}

	SpanStart struct {
		ID     ID `json:"i"`
		Parent ID `json:"p"`

		Location uintptr `json:"l"`

		Started time.Time `json:"s"`
	}

	SpanFinish struct {
		ID      ID            `json:"i"`
		Elapsed time.Duration `json:"e"`
	}

	Message struct {
		Span     ID            `json:"s"`
		Location uintptr       `json:"l"`
		Time     time.Duration `json:"t"`
		Text     string        `json:"m"`
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
