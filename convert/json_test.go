package convert

import (
	"io"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nikandfor/hacked/low"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tlog.app/go/tlog"
	"tlog.app/go/tlog/tlwire"
)

func TestJSON(tb *testing.T) {
	var e tlwire.Encoder
	var b, jb low.Buf

	j := NewJSON(&jb)

	b = e.AppendInt(b[:0], 5)
	jb, _ = j.ConvertValue(jb[:0], b, 0)
	assert.Equal(tb, low.Buf("5"), jb)

	b = e.AppendInt(b[:0], -5)
	jb, _ = j.ConvertValue(jb[:0], b, 0)
	assert.Equal(tb, low.Buf("-5"), jb)
}

func TestJSONLogger(t *testing.T) {
	tm := time.Date(2020, time.December, 25, 22, 8, 13, 0, time.FixedZone("Europe/Moscow", int(3*time.Hour/time.Second)))

	var b low.Buf

	j := NewJSON(&b)
	j.TimeZone = time.FixedZone("MSK", int(3*time.Hour/time.Second))
	j.TimeFormat = time.RFC3339Nano

	l := tlog.New(j)

	tlog.LoggerSetTimeNow(l, func() time.Time { return tm }, tm.UnixNano)

	l.SetLabels(tlog.ParseLabels("a=b,c")...)

	// l.Printw("user labels", "", tlog.Labels{"user_label"})

	l.Printw("message", "str", "arg", "int", 5, "struct", struct {
		A string `json:"a"`
		B int    `tlog:"bb" yaml:"b"`
		C *int   `tlog:"c,omitempty"`
	}{
		A: "A field",
		B: 9,
	})

	exp := `{"_t":"2020-12-25T22:08:13\+03:00","_c":"[\w./-]*json_test.go:\d+","_m":"message","str":"arg","int":5,"struct":{"a":"A field","bb":9},"a":"b","c":""}
`

	exps := strings.Split(exp, "\n")
	ls := strings.Split(string(b), "\n")
	for i := 0; i < len(exps); i++ {
		re := regexp.MustCompile("^" + exps[i] + "$")

		var have string
		if i < len(ls) {
			have = ls[i]
		}

		assert.True(t, re.MatchString(have), "expected\n%s\ngot\n%s", exps[i], have)
	}

	for i := len(exps); i < len(ls); i++ {
		assert.True(t, false, "expected\n%s\ngot\n%s", "", ls[i])
	}
}

func TestJSONRename(t *testing.T) {
	tm := time.Date(2020, time.December, 25, 22, 8, 13, 0, time.FixedZone("Europe/Moscow", int(3*time.Hour/time.Second)))

	var b low.Buf

	j := NewJSON(&b)
	j.TimeZone = time.FixedZone("MSK", int(3*time.Hour/time.Second))
	j.TimeFormat = time.RFC3339Nano

	renamer := simpleTestRenamer()

	j.Rename = renamer.Rename

	l := tlog.New(j)

	tlog.LoggerSetTimeNow(l, func() time.Time { return tm }, tm.UnixNano)

	l.SetLabels(tlog.ParseLabels("a=b,c")...)

	//	l.Printw("user labels", "", tlog.Labels{"user_label"})

	l.Printw("message", "str", "arg", "int", 5, "struct", struct {
		A string `json:"a"`
		B int    `tlog:"bb" yaml:"b"`
		C *int   `tlog:"c,omitempty"`
	}{
		A: "A field",
		B: 9,
	})

	exp := `{"time":"2020-12-25T22:08:13\+03:00","caller":"[\w./-]*json_test.go:\d+","message":"message","str_key":"arg","int":5,"struct":{"a":"A field","bb":9},"L_a":"b","L_c":""}
`

	exps := strings.Split(exp, "\n")
	ls := strings.Split(string(b), "\n")
	for i := 0; i < len(exps); i++ {
		re := regexp.MustCompile("^" + exps[i] + "$")

		var have string
		if i < len(ls) {
			have = ls[i]
		}

		assert.True(t, re.MatchString(have), "expected\n%s\ngot\n%s", exps[i], have)
	}

	for i := len(exps); i < len(ls); i++ {
		assert.True(t, false, "expected\n%s\ngot\n%s", "", ls[i])
	}
}

func BenchmarkJSONConvert(b *testing.B) {
	var buf low.Buf
	var e tlwire.Encoder

	appendMap := func(b []byte, kvs ...interface{}) []byte {
		b = e.AppendMap(b, -1)
		b = tlog.AppendKVs(b, kvs)
		b = e.AppendBreak(b)

		return b
	}

	buf = appendMap(buf, tlog.KeyTimestamp, 10000000000, tlog.KeyEventKind, tlog.EventSpanStart, "a", "b", "c", "d", "e", "d", "h", "g")
	buf = appendMap(buf, tlog.KeySpan, tlog.ID{}, tlog.KeyTimestamp, 10000000000, tlog.KeyMessage, "message text", "arg", "value", "arg2", 5)

	var d tlwire.Decoder

	st := d.Skip(buf, 0)

	w := NewJSON(io.Discard)

	_, err := w.Write(buf[:st])
	require.NoError(b, err)

	_, err = w.Write(buf[st:])
	require.NoError(b, err)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = w.Write(buf[st:])
	}
}

func BenchmarkJSONPrintw(b *testing.B) {
	w := NewJSON(io.Discard)
	l := tlog.New(w)
	//	l.NoCaller = true
	//	l.NoTime = true

	l.SetLabels("a", "b", "c", "d", "e", "f", "g", "h")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		l.Printw("message", "a", i+1000, "b", i+1000)
	}
}
