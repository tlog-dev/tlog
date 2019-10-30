package parse

import (
	"time"

	"github.com/nikandfor/tlog"
)

type (
	ID     = tlog.ID
	Labels = tlog.Labels

	Location struct {
		PC   uintptr `json:"pc"`
		Name string  `json:"n"`
		File string  `json:"f"`
		Line int     `json:"l"`
	}

	Span struct {
		ID     ID
		Parent ID

		Location uintptr

		Started time.Time
		Elapsed time.Duration
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

	Reader interface {
		Read() (interface{}, error)
	}
)
