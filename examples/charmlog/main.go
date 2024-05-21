package main

import (
	"fmt"
	"os"
	"time"

	"tlog.app/go/tlog"
	"tlog.app/go/tlog/ext/tlwtag"
	"tlog.app/go/tlog/tlwire"
)

const (
	Cups = tlwtag.SemanticUserBase + iota
)

type cup int

func (c cup) String() string {
	s := fmt.Sprintf("%d cup", c)
	if c > 1 {
		s += "s"
	}
	return s
}

// TlogAppend overrides the default String() encoding.
// It's useful for making it machine-readable.
func (c cup) TlogAppend(b []byte) []byte {
	var e tlwire.Encoder
	b = e.AppendSemantic(b, Cups)
	b = e.AppendInt(b, int(c))
	return b
}

func before() {
	w := tlog.NewConsoleWriter(os.Stderr, tlog.Ltime|tlog.Lloglevel)

	tlog.DefaultLogger = tlog.New(w)

	tlog.SetVerbosity("oven,ingredients") // enable debug logs; precisely
}

func startOven(degree int) {
	tlog.V("oven").NewMessage(1, tlog.ID{}, "Starting oven",
		"temperature", degree,
		tlog.KeyLogLevel, tlog.Debug, // not idiomatic, but possible
	)
}

// charmlog example rewritten in tlog
//
// https://github.com/charmbracelet/log/blob/2d80948d38ad30d727aed3a1984f4a4911203019/examples/app/main.go
func main() {
	before()

	var (
		butter    = cup(1)
		chocolate = cup(2)
		flour     = cup(3)
		sugar     = cup(5)
		temp      = 375
		bakeTime  = 10 * time.Minute
	)

	startOven(temp)
	time.Sleep(time.Second)

	// Will not be printed if "ingredients" debug topic is not selected.
	// No allocs made in both cases, that is why the interface is a bit nerdy.
	tlog.V("ingredients").Printw("Mixing ingredients", "ingredients",
		tlog.RawTag(tlwire.Map, 4), // idiomatic way is to use flat structure,
		"butter", butter,           // but possible to have as complex structure as you need
		"chocolate", chocolate,
		"flour", flour,
		"sugar", sugar,

		"", tlog.LogLevel(-3), // not idiomatic, but can even have multiple levels of debug (this is third)
	)

	time.Sleep(time.Second)

	if sugar > 2 {
		tlog.Printw("That's a lot of sugar", "amount", sugar, "", tlog.Warn) // the "" key means it's guessed by value type
	}

	tlog.Printw("Baking cookies", "time", bakeTime)

	time.Sleep(2 * time.Second)

	tlog.Printw("Increasing temperature", "amount", 300)

	temp += 300
	time.Sleep(time.Second)

	if temp > 500 {
		tlog.Printw("Oven is too hot", "temperature", temp, "", tlog.Error)
		tlog.Printw("The kitchen is on fire 🔥", "", tlog.Fatal)
		os.Exit(1)
	}
}
