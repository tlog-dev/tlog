package main

import (
	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/low"
	"github.com/nikandfor/tlog/wire"
	"github.com/nikandfor/tlog/writer"
)

func main() {
	//	var buf bytes.Buffer
	//	jw := writer.NewJSON(&buf)

	l := tlog.New(writer.NewConsole(tlog.Stderr, writer.LdetFlags))

	// usual way
	l.Printw("message", "int", 100, "str", "string")

	// the same output but customizable
	l.Event([]wire.Tag{
		{T: wire.Time, V: low.UnixNano()},
		//	{T: wire.Location, V: tlog.PC(0)},
		{T: wire.Message, V: "message"},
	}, []interface{}{
		"int", 100,
		"str", "string",
	})

	// empty event
	l.Event(nil, nil)

	// without time
	tr := tlog.Span{Logger: l, ID: l.NewID()}
	tr.Event([]wire.Tag{
		//	{T: wire.Location, V: tlog.Caller(0)},
		{T: wire.Type, V: 's'},
	}, nil)

	// without location
	tr.Event([]wire.Tag{
		{T: wire.Time, V: low.UnixNano()},
		{T: wire.Message, V: "message"},
	}, nil)

	hotCode(tr, 300)

	tr.Finish()

	//	_, _ = buf.WriteTo(tlog.Stderr)
}

var hotCodeLoc tlog.PC

func hotCode(tr tlog.Span, arg int) {
	tr.Event([]wire.Tag{
		{T: wire.Time, V: low.UnixNano()},
	}, []interface{}{"arg", arg})

	//		CallerOnce(0, &hotCodeLoc). // faster than Caller
}
