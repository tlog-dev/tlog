package convert

import (
	"io/ioutil"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/low"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSON(t *testing.T) {
	tm := time.Date(2020, time.December, 25, 22, 8, 13, 0, time.FixedZone("Europe/Moscow", int(3*time.Hour/time.Second)))

	var b low.Buf

	j := NewJSONWriter(&b)
	j.AttachLabels = true
	j.TimeZone = time.FixedZone("MSK", int(3*time.Hour/time.Second))
	j.TimeFormat = time.RFC3339Nano

	l := tlog.New(j)

	tlog.TestSetTime(l, func() time.Time { return tm }, tm.UnixNano)

	l.SetLabels(tlog.Labels{"a=b", "c"})

	l.Printw("user labels", "", tlog.Labels{"user_label"})

	l.Printw("message", "str", "arg", "int", 5, "struct", struct {
		A string `json:"a"`
		B int    `tlog:"bb" yaml:"b"`
		C *int   `tlog:"c,omitempty"`
	}{
		A: "A field",
		B: 9,
	})

	exp := `{"t":"2020-12-25T22:08:13\+03:00","T":"L","L":\["a=b","c"\]}
{"t":"2020-12-25T22:08:13\+03:00","c":"[\w./-]*json_test.go:\d+","m":"user labels","L":\["user_label"\],"L":\["a=b","c"\]}
{"t":"2020-12-25T22:08:13\+03:00","c":"[\w./-]*json_test.go:\d+","m":"message","str":"arg","int":5,"struct":{"a":"A field","bb":9},"L":\["a=b","c"\]}
`

	exps := strings.Split(exp, "\n")
	ls := strings.Split(string(b), "\n")
	for i := 0; i < len(exps); i++ {
		re := regexp.MustCompile(exps[i])

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

func BenchmarkJSON(b *testing.B) {
	var buf low.Buf
	var e tlog.Encoder

	buf = e.AppendMap(buf, []interface{}{tlog.KeyTime, 10000000000, tlog.KeyEventType, tlog.EventLabels, tlog.KeyLabels, tlog.Labels{"a=b", "c=d", "e=f", "g=h"}})
	buf = e.AppendMap(buf, []interface{}{tlog.KeySpan, tlog.ID{}, tlog.KeyTime, 10000000000, tlog.KeyMessage, "message text", "arg", "value", "arg2", 5})

	var d tlog.Decoder
	d.ResetBytes(buf)

	st := d.Skip(0)

	w := NewJSONWriter(ioutil.Discard)

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
