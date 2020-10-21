package tlwire

import (
	"io"

	"github.com/nikandfor/tlog"
)

type (
	ID    = tlog.ID
	Type  = tlog.Type
	Level = tlog.Level

	Reader struct {
		io.Reader
	}

	Event struct {
		Span  ID
		Time  int64
		PC    int32
		Type  Type
		Level Level
	}
)
