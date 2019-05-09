package tlog

import (
	"bytes"
	"math/rand"
	"testing"
	"time"

	"github.com/nikandfor/json"
	"github.com/stretchr/testify/assert"
)

type testType struct{}

func (*testType) Method(t *testing.T, l *Logger, id FullID) {
	s := l.Spawn(id)
	defer s.Finish()

	s.Logf("sub.meth. par %v", id)

	l.Logf("usual log from span %v", id)
}

func (testType) Method2(t *testing.T, l *Logger, id FullID) {
	s := l.Spawn(id)
	defer s.Finish()

	s.Logf("sub sub. par %v", id)
}

func testWriterInsideSpan(t *testing.T, l *Logger) {
	var tt testType

	sub := func(id FullID) {
		s := l.Spawn(id)
		defer s.Finish()

		s.Logf("sub before")

		tt.Method(t, l, s.ID)

		tt.Method2(t, l, s.ID)

		s.Logf("sub after")
	}

	s := l.Start()
	defer s.Finish()

	s.Logf("before")

	sub(s.ID)

	s.Logf("after")
}

func TestJSONWriter(t *testing.T) {
	t.Skip()

	defer func(f func() time.Time) {
		now = f
	}(now)
	tm, _ := time.Parse("2006-01-02_15:04:05.000000", "2019-05-09_17:43:00.122044")
	now = func() time.Time {
		tm = tm.Add(10 * time.Millisecond)
		return tm
	}
	rnd = rand.New(rand.NewSource(0))

	var buf bytes.Buffer
	buf.WriteByte('\n')

	jw := json.NewStreamWriter(&buf)
	w := NewJSONWriter(jw)
	l := NewLogger(w)

	testWriterInsideSpan(t, l)

	err := w.Flush()
	assert.NoError(t, err)

	assert.Equal(t, `
{"loc":{"pc":6888207,"f":"github.com/nikandfor/tlog/json_writer_test.go","n":"github.com/nikandfor/tlog.(*testType).Method","e":15,"l":21}}
{"l":{"st":1557423780192044000,"loc":6888207,"msg":usual log from span {78fc2ffac2fd9401 53f65ff94f6ec873}}}
{"loc":{"pc":6887552,"f":"github.com/nikandfor/tlog/json_writer_test.go","n":"github.com/nikandfor/tlog.(*testType).Method","e":15,"l":15}}
{"loc":{"pc":6888022,"f":"github.com/nikandfor/tlog/json_writer_test.go","n":"github.com/nikandfor/tlog.(*testType).Method","e":15,"l":19}}
{"s":{"tr":"78fc2ffac2fd9401","id":"06f4bd2ae8eea562","par":"53f65ff94f6ec873","loc":6887552,"st":1557423780172044000,"el":30000000,"logs":[{"st":10000000,"loc":6888022,"msg":sub.meth. par {78fc2ffac2fd9401 53f65ff94f6ec873}}]}}
{"loc":{"pc":6888352,"f":"github.com/nikandfor/tlog/json_writer_test.go","n":"github.com/nikandfor/tlog.testType.Method2","e":24,"l":24}}
{"loc":{"pc":6888818,"f":"github.com/nikandfor/tlog/json_writer_test.go","n":"github.com/nikandfor/tlog.testType.Method2","e":24,"l":28}}
{"s":{"tr":"78fc2ffac2fd9401","id":"2f0d18fb750b2d4a","par":"53f65ff94f6ec873","loc":6888352,"st":1557423780212044000,"el":20000000,"logs":[{"st":10000000,"loc":6888818,"msg":sub sub. par {78fc2ffac2fd9401 53f65ff94f6ec873}}]}}
{"loc":{"pc":6893056,"f":"github.com/nikandfor/tlog/json_writer_test.go","n":"github.com/nikandfor/tlog.testWriterInsideSpan.func1","e":34,"l":34}}
{"loc":{"pc":6893428,"f":"github.com/nikandfor/tlog/json_writer_test.go","n":"github.com/nikandfor/tlog.testWriterInsideSpan.func1","e":34,"l":38}}
{"loc":{"pc":6893608,"f":"github.com/nikandfor/tlog/json_writer_test.go","n":"github.com/nikandfor/tlog.testWriterInsideSpan.func1","e":34,"l":44}}
{"s":{"tr":"78fc2ffac2fd9401","id":"53f65ff94f6ec873","par":"1f5b0412ffd341c0","loc":6893056,"st":1557423780152044000,"el":100000000,"logs":[{"st":10000000,"loc":6893428,"msg":sub before},{"st":90000000,"loc":6893608,"msg":sub after}]}}
{"loc":{"pc":6888944,"f":"github.com/nikandfor/tlog/json_writer_test.go","n":"github.com/nikandfor/tlog.testWriterInsideSpan","e":31,"l":31}}
{"loc":{"pc":6889263,"f":"github.com/nikandfor/tlog/json_writer_test.go","n":"github.com/nikandfor/tlog.testWriterInsideSpan","e":31,"l":50}}
{"loc":{"pc":6889361,"f":"github.com/nikandfor/tlog/json_writer_test.go","n":"github.com/nikandfor/tlog.testWriterInsideSpan","e":31,"l":54}}
{"s":{"tr":"78fc2ffac2fd9401","id":"1f5b0412ffd341c0","loc":6888944,"st":1557423780132044000,"el":140000000,"logs":[{"st":10000000,"loc":6889263,"msg":before},{"st":130000000,"loc":6889361,"msg":after}]}}
`, buf.String())
}
