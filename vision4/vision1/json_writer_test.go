package tlog

import (
	"math/rand"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func testRandID() func() ID {
	var mu sync.Mutex
	rnd := rand.New(rand.NewSource(0))

	return func() (id ID) {
		mu.Lock()
		for id == (ID{}) {
			_, _ = rnd.Read(id[:])
		}
		mu.Unlock()
		return
	}
}

func TestJSONWriter(t *testing.T) {
	tm := time.Date(2020, time.October, 19, 11, 26, 25, 0, time.Local)
	now = func() time.Time {
		tm = tm.Add(time.Second)
		return tm
	}

	var buf bufWriter

	w := NewJSONWriter(&buf)
	l := &Logger{NewID: testRandID()}

	w.Write(&Event{Time: now()})
	w.Write(&Event{Type: 'T', Level: 3})
	w.Write(&Event{Type: '"', Level: -1})
	w.Write(&Event{Level: 99})
	w.Write(&Event{Level: -128})
	w.Write(&Event{PC: Caller(0)})
	w.Write((&Event{}).Labels(Labels{"a=b", "c"}))
	w.Write((&Event{Logger: l}).Tp('s').NewID().Now())
	w.Write((&Event{}).Any("my", "own"))
	w.Write((&Event{}).Tp('Q').Caller(0).Any("my", "own"))
	w.Write((&Event{}).Dict(D{"T": 1, "t": "str", "pc": "location", "s": 123}))

	re := `{"t":1603095986000000000}
{"T":"T","l":3}
{"T":"\\"","l":-1}
{"l":99}
{"l":-128}
{"pc":\d+}
{"L":\["a=b","c"\]}
{"s":"0194fdc2fa2ffcc041d3ff12045b73c8","t":1603095987000000000,"T":"s"}
{"my":"own"}
{"T":"Q","pc":\d+,"my":"own"}
{"T":1,"pc":"location","s":123,"t":"str"}
`

	testCompareOutput(t, re, string(buf))
}

func testCompareOutput(t *testing.T, re, got string) {
	t.Helper()

	bl := strings.Split(got, "\n")
	rels := strings.Split(re, "\n")

	for i, rel := range rels {
		if i == len(bl) {
			break
		}

		ok, err := regexp.Match("^"+rel+"$", []byte(bl[i]))
		assert.NoError(t, err)
		assert.True(t, ok, "expected:\n%v\nactual:\n%v\n", rel, bl[i])
	}

	for i := len(bl); i < len(rels); i++ {
		t.Errorf("expected:\n%v\nactual: no more data\n", rels[i])
	}

	for i := len(rels); i < len(bl); i++ {
		t.Errorf("expected: no more data\nactual:\n%v\n", bl[i])
	}
}
