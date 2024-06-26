package convert

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nikandfor/hacked/low"
	"github.com/stretchr/testify/assert"

	"tlog.app/go/tlog"
	"tlog.app/go/tlog/tlwire"
)

func TestLogfmt(tb *testing.T) {
	var e tlwire.Encoder
	var b, jb low.Buf

	j := NewLogfmt(&jb)

	b = e.AppendInt(b[:0], 5)
	jb, _ = j.ConvertValue(jb[:0], b, nil, 0)
	assert.Equal(tb, low.Buf("5"), jb)

	b = e.AppendInt(b[:0], -5)
	jb, _ = j.ConvertValue(jb[:0], b, nil, 0)
	assert.Equal(tb, low.Buf("-5"), jb)
}

func TestLogfmtFormatDuration(tb *testing.T) {
	var e tlwire.Encoder
	var b, jb low.Buf

	j := NewLogfmt(&jb)

	b = e.AppendDuration(b[:0], 5300*time.Microsecond)
	jb, _ = j.ConvertValue(jb[:0], b, nil, 0)
	assert.Equal(tb, low.Buf("5.3ms"), jb)

	j.DurationFormat = "%.3fms"
	j.DurationDiv = time.Millisecond

	jb, _ = j.ConvertValue(jb[:0], b, nil, 0)
	assert.Equal(tb, low.Buf("5.300ms"), jb)

	j.DurationFormat = ""

	jb, _ = j.ConvertValue(jb[:0], b, nil, 0)
	assert.Equal(tb, low.Buf("5300000"), jb)
}

func TestLogfmtSubObj(t *testing.T) {
	testLogfmtObj(t, false)
}

func TestLogfmtFlatObj(t *testing.T) {
	testLogfmtObj(t, true)
}

func testLogfmtObj(t *testing.T, flat bool) {
	tm := time.Date(2020, time.December, 25, 22, 8, 13, 0, time.FixedZone("Europe/Moscow", int(3*time.Hour/time.Second)))

	var b low.Buf

	j := NewLogfmt(&b)
	j.SubObjects = !flat
	j.QuoteEmptyValue = true
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
	if flat {
		exp = `_t="2020-12-25T22:08:13\+03:00"  _c="[\w./-]*logfmt_test.go:\d+"  _m=message  str=arg  int=5  struct.a="A field"  struct.bb=9  a=b  c=""
`
	}

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
	j.SubObjects = true
	j.QuoteEmptyValue = true
	j.TimeZone = time.FixedZone("MSK", int(3*time.Hour/time.Second))
	j.TimeFormat = time.RFC3339Nano

	renamer := simpleTestRenamer()

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

func TestLogfmtKeyWithSpace(t *testing.T) {
	var e tlwire.Encoder
	var b low.Buf

	j := NewLogfmt(&b)
	j.QuoteEmptyValue = true

	_, err := j.Write(tlog.AppendKVs(e, nil, []interface{}{tlog.RawTag(tlwire.Map, 1), "key with spaces", "value"}))
	assert.NoError(t, err)
	assert.Equal(t, `"key with spaces"=value`+"\n", string(b))
}

func simpleTestRenamer() SimpleRenamer {
	return SimpleRenamer{
		Rules: map[string]SimpleRenameRule{
			tlog.KeyEventKind: {Tags: []TagSub{{tlwire.Semantic, tlog.WireEventKind}}, Key: "kind"},
			tlog.KeyTimestamp: {Tags: []TagSub{{tlwire.Semantic, tlwire.Time}}, Key: "time"},
			tlog.KeyCaller:    {Tags: []TagSub{{tlwire.Semantic, tlwire.Caller}}, Key: "caller"},
			tlog.KeyMessage:   {Tags: []TagSub{{tlwire.Semantic, tlog.WireMessage}}, Key: "message"},

			"str": {Tags: []TagSub{{Tag: tlwire.String}}, Key: "str_key"},
		},
		Fallback: func(b, p, k []byte, i int) ([]byte, bool) {
			var d tlwire.Decoder

			tag, sub, _ := d.Tag(p, i)

			if tag != tlwire.Semantic || sub != tlog.WireLabel {
				return b, false
			}

			b = append(b, "L_"...)
			b = append(b, k...)

			return b, true
		},
	}
}
