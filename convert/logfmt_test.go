package convert

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/low"
	"github.com/nikandfor/tlog/tlwire"
	"github.com/stretchr/testify/assert"
)

func TestLogfmt(t *testing.T) {
	tm := time.Date(2020, time.December, 25, 22, 8, 13, 0, time.FixedZone("Europe/Moscow", int(3*time.Hour/time.Second)))

	var b low.Buf

	j := NewLogfmt(&b)
	j.TimeZone = time.FixedZone("MSK", int(3*time.Hour/time.Second))
	j.TimeFormat = time.RFC3339Nano

	l := tlog.New(j)

	tlog.LoggerSetTimeNow(l, func() time.Time { return tm }, tm.UnixNano)

	l.SetLabels(tlog.ParseLabels("a=b,c")...)

	l.Printw("message", "str", "arg", "int", 5, "struct", struct {
		A string `json:"a"`
		B int    `tlog:"bb" yaml:"b"`
		C *int   `tlog:"c,omitempty"`
	}{
		A: "A field",
		B: 9,
	})

	exp := `_t="2020-12-25T22:08:13\+03:00"  _c="[\w./-]*logfmt_test.go:\d+"  _m=message  str=arg  int=5  struct={a="A field" bb=9}  a=b  c=""
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

func TestLogfmtRename(t *testing.T) {
	tm := time.Date(2020, time.December, 25, 22, 8, 13, 0, time.FixedZone("Europe/Moscow", int(3*time.Hour/time.Second)))

	var b low.Buf

	j := NewLogfmt(&b)
	j.TimeZone = time.FixedZone("MSK", int(3*time.Hour/time.Second))
	j.TimeFormat = time.RFC3339Nano

	renamer := Renamer{
		Rules: map[string]RenameRule{
			tlog.KeyEventKind: {Tags: []TagSub{{tlwire.Semantic, tlog.WireEventKind}}, Key: "kind"},
			tlog.KeyTimestamp: {Tags: []TagSub{{tlwire.Semantic, tlwire.Time}}, Key: "time"},
			tlog.KeyCaller:    {Tags: []TagSub{{tlwire.Semantic, tlwire.Caller}}, Key: "caller"},
			tlog.KeyMessage:   {Tags: []TagSub{{tlwire.Semantic, tlog.WireMessage}}, Key: "message"},

			"str": {Tags: []TagSub{{Tag: tlwire.String}}, Key: "str_key"},
		},
		Fallback: func(b, p, k []byte, i int) ([]byte, bool) {
			var d tlwire.Decoder

			tag, sub, i := d.Tag(p, i)

			if tag != tlwire.Semantic || sub != tlog.WireLabel {
				return b, false
			}

			b = append(b, "L_"...)
			b = append(b, k...)

			return b, true
		},
	}

	j.Rename = renamer.Rename

	l := tlog.New(j)

	tlog.LoggerSetTimeNow(l, func() time.Time { return tm }, tm.UnixNano)

	l.SetLabels(tlog.ParseLabels("a=b,c")...)

	l.Printw("message", "str", "arg", "int", 5, "struct", struct {
		A string `json:"a"`
		B int    `tlog:"bb" yaml:"b"`
		C *int   `tlog:"c,omitempty"`
	}{
		A: "A field",
		B: 9,
	})

	exp := `time="2020-12-25T22:08:13\+03:00"  caller="[\w./-]*logfmt_test.go:\d+"  message=message  str_key=arg  int=5  struct={a="A field" bb=9}  L_a=b  L_c=""
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
