package tlsyslog

import "log/syslog"

type (
	Writer struct {
		w *syslog.Writer
	}
)
