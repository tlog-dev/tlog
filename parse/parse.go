package parse

import (
	"time"

	"github.com/nikandfor/tlog"
)

type (
	ID     = tlog.ID
	Labels = tlog.Labels

	Location struct {
		PC   uintptr
		Name string
		File string
		Line int
	}

	Span struct {
		ID     ID
		Parent ID

		Location uintptr

		Started time.Time
		Elapsed time.Duration

		Flags int
	}

	SpanFinish struct {
		ID      ID
		Elapsed time.Duration
		Flags   int
	}

	Message struct {
		Span     ID
		Location uintptr
		Time     time.Duration
		Text     string
	}
)
