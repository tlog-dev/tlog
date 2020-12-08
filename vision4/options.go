package tlog

func WithNoCaller(l *Logger) {
	l.NoCaller = true
}

func WithNoTime(l *Logger) {
	l.NoTime = true
}
