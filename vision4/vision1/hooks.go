package tlog

import "context"

func AddNow(ctx context.Context, ev *Event) error {
	ev.Time = now()

	return nil
}

func AddCaller(ctx context.Context, ev *Event) error {
	if ev.Type == 's' {
		ev.PC = Funcentry(3)
	} else {
		ev.PC = Caller(3)
	}

	return nil
}
