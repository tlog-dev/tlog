package main

import (
	"context"
	"strconv"

	"github.com/nikandfor/tlog"
)

func main() {
	ctx := context.Background()

	tr := tlog.Start()
	defer tr.Finish()

	usualCode(tlog.ContextWithSpan(ctx, tr), 1, 2.3)

	var buf []byte

	buf = hotCode(tr, 4, 5.6, buf)

	buf = fire(tr, 7, 8.9, buf)

	// benchmark, measure and analyze to go beyond
}

func usualCode(ctx context.Context, a1 int, a2 float64) {
	tr := tlog.SpawnFromContext(ctx)
	defer tr.Finish()

	tr.Printf("usual  log int %v", a1)

	// work

	tr.Printf("usual  log float %v", a2)
}

func hotCode(tr tlog.Span, a1 int, a2 float64, buf []byte) []byte {
	// use the same span

	buf = append(buf[:0], "hotter log int "...)
	buf = strconv.AppendInt(buf, int64(a1), 10)

	tr.PrintBytes(0, buf)

	// work

	buf = append(buf[:0], "hotter float "...)
	buf = strconv.AppendFloat(buf, a2, 'f', -1, 64)

	tr.PrintBytes(0, buf)

	return buf // reuse buffer
}

var firePC tlog.PC

func fire(tr tlog.Span, a1 int, a2 float64, buf []byte) []byte {
	// use the same span

	buf = append(buf[:0], "fire   log int "...)
	buf = strconv.AppendInt(buf, int64(a1), 10)

	// work

	buf = append(buf, "  float "...)
	buf = strconv.AppendFloat(buf, a2, 'f', -1, 64)

	if firePC == 0 {
		firePC = tlog.Caller(0)
	}

	tr.Logger.Message(tlog.Message{
		PC: fireFrame,
		// Time: time.Now().UnixNano(),
		Text: tlog.UnsafeBytesToString(buf),
	}, tr.ID)

	return buf // reuse buffer
}
