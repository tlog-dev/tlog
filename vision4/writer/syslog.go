package writer

import "log/syslog"

type (
	Syslog struct {
		*syslog.Writer
	}
)

func (w *Syslog) Write(p []byte) (int, error) {
	panic("nope")
}
