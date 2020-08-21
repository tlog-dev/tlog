package main

import (
	"bytes"
	"context"
	"strconv"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/parse"
)

func main() {
	ctx := context.Background()

	tr := tlog.Start()
	defer tr.Finish()

	usualCode(tlog.ContextWithSpan(ctx, tr), 1, 2.3)

	var buf []byte

	buf = hotCode(tr, 4, 5.6, buf)

	buf = fire(tr, 7, 8.9, buf)

	// dedicated non thread-safe writer
	cw := tlog.NewConsoleWriter(tlog.Stderr, tlog.LstdFlags)

	buf = madness(cw, tr.ID, 11, 22.33, buf)

	// use more efficient writer
	var file bytes.Buffer
	pw := tlog.NewProtoWriter(&file)

	buf = madness(pw, tr.ID, 44, 55.66, buf)

	// convert later by "tlog convert" command
	pr := parse.NewProtoReader(&file)
	ccw := parse.NewAnyWiter(cw)

	err := parse.Copy(ccw, pr)
	if err != nil {
		tlog.Fatalf("convert: %v", err)
	}

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

	//	tr.PrintRaw(0, buf) // combine messages

	// work

	buf = append(buf, "  float "...)
	buf = strconv.AppendFloat(buf, a2, 'f', -1, 64)

	tr.PrintRaw(0, buf)

	return buf // reuse buffer
}

var fireLocation tlog.Location

func fire(tr tlog.Span, a1 int, a2 float64, buf []byte) []byte {
	// use the same span

	buf = append(buf[:0], "fire   log int "...)
	buf = strconv.AppendInt(buf, int64(a1), 10)

	// work

	buf = append(buf, "  float "...)
	buf = strconv.AppendFloat(buf, a2, 'f', -1, 64)

	if fireLocation == 0 {
		fireLocation = tlog.Caller(0)
	}

	tr.Logger.Message(tlog.Message{
		Location: fireLocation,
		// Time: time.Now().UnixNano(),
		Format: tlog.UnsafeBytesToString(buf),
	}, tr.ID)

	return buf // reuse buffer
}

var madnessLocation tlog.Location

func madness(w tlog.Writer, id tlog.ID, a1 int, a2 float64, buf []byte) []byte {
	// use the same span

	buf = append(buf[:0], "fire   log int "...)
	buf = strconv.AppendInt(buf, int64(a1), 10)

	// work

	buf = append(buf, "  float "...)
	//	buf = strconv.AppendFloat(buf, a2, 'f', -1, 64)
	buf = strconv.AppendInt(buf, int64(a2), 10)
	buf = append(buf, '.')
	buf = strconv.AppendInt(buf, int64(a2*(1e6))%1e6, 10)

	if madnessLocation == 0 {
		madnessLocation = tlog.Caller(0)
	}

	w.Message(tlog.Message{
		Location: madnessLocation,
		// Time: time.Now().UnixNano(),
		Format: tlog.UnsafeBytesToString(buf),
	}, id)

	return buf // reuse buffer
}
