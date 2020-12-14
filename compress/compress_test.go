package compress

import (
	"bytes"
	"encoding/hex"
	"io"
	"io/ioutil"
	"os"
	"testing"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/low"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testData   []byte
	testOff    []int
	testsCount int
)

func TestCompare(t *testing.T) {
	const B = 32

	w := newEncoder(nil, B, 0)

	st, end := w.compare([]byte("1234567890"), 0, 0)
	assert.Equal(t, int64(0), st)
	assert.Equal(t, int64(0), end)

	copy(w.block, "12345678901234567890")
	w.pos = int64(len(w.block)) + 2

	t.Logf("block: %d %q", len(w.block), w.block)

	st, end = w.compare([]byte("1234567890"), 0, 0)
	assert.Equal(t, int64(0), st)
	assert.Equal(t, int64(10), end)

	st, end = w.compare([]byte("123456789012"), 2, 2)
	assert.Equal(t, int64(0), st)
	assert.Equal(t, int64(12), end)

	copy(w.block, "4567890             ")
	copy(w.block[B-3:], "123")

	st, end = w.compare([]byte("1234567890"), 1, B-2)
	assert.Equal(t, int64(B-3), st)
	assert.Equal(t, int64(B+7), end)

	copy(w.block, "890             ")
	copy(w.block[B-len("bcdef1234567"):], "bcdef1234567")

	st, end = w.compare([]byte("++++abcdef1234567890qw"), int64(len("++++abcdef12345678")), B+1)
	assert.Equal(t, int64(B-len("bcdef1234567")), st)
	assert.Equal(t, int64(B+3), end)
}

func TestLiteral(t *testing.T) {
	const B = 32

	tl = tlog.NewTestLogger(t, "", nil)

	//tl.Writer = tlog.NewTeeWriter(tl.Writer, tlog.NewDumper(os.Stderr))

	var buf low.Buf
	st := 0

	w := newEncoder(&buf, B, 1)

	n, err := w.Write([]byte("very_first_message"))
	assert.Equal(t, 18, n)
	assert.NoError(t, err)

	tl.Printf("buf pos %x ht %x\n%v", w.pos, w.ht, hex.Dump(w.block))
	tl.Printf("res\n%v", hex.Dump(buf[st:]))
	st = len(buf)

	r := &Decoder{
		b:   buf,
		end: int64(len(buf)),
	}

	p := make([]byte, 100)

	tl.Printf("*** read back ***")

	n, err = r.Read(p[:10])
	assert.Equal(t, 10, n)
	assert.NoError(t, err)
	assert.Equal(t, []byte("very_first"), p[:n])

	n, err = r.Read(p[:10])
	assert.Equal(t, 8, n)
	assert.Equal(t, io.EOF, err)
	assert.Equal(t, []byte("_message"), p[:n])
}

func TestCopy(t *testing.T) {
	const B = 32

	tl = tlog.NewTestLogger(t, "", nil)

	//tl.Writer = tlog.NewTeeWriter(tl.Writer, tlog.NewDumper(os.Stderr))

	var buf low.Buf
	st := 0

	w := newEncoder(&buf, B, 1)

	n, err := w.Write([]byte("prefix_1234_suffix"))
	assert.Equal(t, 18, n)
	assert.NoError(t, err)

	tl.Printf("buf pos %x ht %x\n%v", w.pos, w.ht, hex.Dump(w.block))
	tl.Printf("res\n%v", hex.Dump(buf[st:]))
	st = len(buf)

	n, err = w.Write([]byte("prefix_567_suffix"))
	assert.Equal(t, 17, n)
	assert.NoError(t, err)

	tl.Printf("buf  pos %x ht %x\n%v", w.pos, w.ht, hex.Dump(w.block))
	tl.Printf("res\n%v", hex.Dump(buf[st:]))
	st = len(buf)

	r := &Decoder{
		b:   buf,
		end: int64(len(buf)),
	}

	p := make([]byte, 100)

	tl.Printf("*** read back ***")

	n, err = r.Read(p[:10])
	assert.Equal(t, 10, n)
	assert.NoError(t, err)
	assert.Equal(t, []byte("prefix_123"), p[:n])

	tl.Printf("buf  pos %x\n%v", r.pos, hex.Dump(r.block))

	n, err = r.Read(p[:10])
	assert.Equal(t, 10, n)
	assert.NoError(t, err)
	assert.Equal(t, []byte("4_suffixpr"), p[:n])

	tl.Printf("buf  pos %x\n%v", r.pos, hex.Dump(r.block))

	n, err = r.Read(p[:30])
	assert.Equal(t, 15, n)
	assert.Equal(t, io.EOF, err)
	assert.Equal(t, []byte("efix_567_suffix"), p[:n])

	tl.Printf("buf  pos %x\n%v", r.pos, hex.Dump(r.block))

	tl.Printw("compression", "ratio", float64(len(buf))/float64(18+17))
}

func TestDumpFile(t *testing.T) {
	const MaxEvents = 100000

	f, err := os.Create("/tmp/seen.log")
	require.NoError(t, err)

	tl = nil
	tl = tlog.NewTestLogger(t, "",
		//	tlog.Stderr,
		//	nil,
		f,
	)

	data, err := ioutil.ReadFile("../log.tlog")
	if err != nil {
		t.Skipf("open test data: %v", err)
	}

	dec := tlog.NewDecoderBytes(data)

	var dump, encoded low.Buf

	d := NewDumper(&dump)
	w := newEncoder(tlog.NewTeeWriter(&encoded, d), 32*1024, 6)

	st := 0
	for n := 0; n < MaxEvents && st < len(data); n++ {
		end := dec.Skip(st)

		m, err := w.Write(data[st:end])
		if !assert.NoError(t, err, "%v bytes written", m) {
			break
		}

		st = end
	}

	//	t.Logf("dump:\n%s", dump)
	err = ioutil.WriteFile("/tmp/seen.dump", dump, 0644)
	require.NoError(t, err)
	t.Logf("block size: %x", len(w.block))
	t.Logf("writer pos: %x", w.pos)

	r := NewDecoderBytes(encoded)

	decoded, err := ioutil.ReadAll(r)
	assert.NoError(t, err)

	assert.Equal(t, data[:len(decoded)], decoded)
}

func TestDumpOnelineText(t *testing.T) {
	t.Skip()

	tl = tlog.NewTestLogger(t, "", nil)

	var dump, text low.Buf

	d := NewDumper(&dump)
	e := newEncoder(d, 1*1024, 2)

	cw := tlog.NewConsoleWriter(tlog.NewTeeWriter(e, &text), tlog.LstdFlags)

	l := tlog.New(cw)
	tr := l.Start("span_name")

	types := []string{"type_a", "value_b", "qweqew", "asdads"}

	for i := 0; i < 20; i++ {
		//	tr := l.Start("span_name")
		tr.Printw("some example message", "i", i, "type", types[i%len(types)])
		//	tr.Finish()
	}

	t.Logf("text:\n%s", text)
	t.Logf("dump:\n%s", dump)
}

func BenchmarkLogCompressOneline(b *testing.B) {
	tl = nil

	b.ReportAllocs()

	var buf low.Buf
	w := NewEncoder(&buf, 128*1024)
	var c tlog.CountableIODiscard

	l := tlog.New(io.MultiWriter(w, &c))
	tr := l.Start("span_name")

	types := []string{"type_a", "value_b", "qweqew", "asdads"}

	for i := 0; i < b.N; i++ {
		//	tr := l.Start("span_name")
		tr.Printw("some example message", "i", i, "type", types[i%len(types)])
		//	tr.Finish()
	}

	b.SetBytes(c.Bytes / int64(b.N))

	b.ReportMetric(float64(len(buf))/float64(c.Bytes), "ratio")
}

func BenchmarkLogCompressOnelineText(b *testing.B) {
	tl = nil

	b.ReportAllocs()

	var buf low.Buf
	w := NewEncoder(&buf, 128*1024)
	var c tlog.CountableIODiscard

	cw := tlog.NewConsoleWriter(io.MultiWriter(w, &c), tlog.LstdFlags)

	l := tlog.New(cw)
	tr := l.Start("span_name")

	types := []string{"type_a", "value_b", "qweqew", "asdads"}

	for i := 0; i < b.N; i++ {
		//	tr := l.Start("span_name")
		tr.Printw("some example message", "i", i, "type", types[i%len(types)])
		//	tr.Finish()
	}

	b.SetBytes(c.Bytes / int64(b.N))

	b.ReportMetric(float64(len(buf))/float64(c.Bytes), "ratio")
}

func BenchmarkEncodeFile(b *testing.B) {
	tl = nil
	//	tl = tlog.NewTestLogger(b, "", os.Stderr)

	err := loadTestFile(b, "../log.tlog")
	if err != nil {
		b.Skipf("loading data: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	var c tlog.CountableIODiscard
	w := newEncoder(&c, 1024*1024, 6)

	//	b.Logf("block %x  ht %x (%x * %x)", len(w.block), len(w.ht)*int(unsafe.Sizeof(w.ht[0])), len(w.ht), unsafe.Sizeof(w.ht[0]))

	written := 0
	for i := 0; i < b.N; i++ {
		j := i % testsCount
		msg := testData[testOff[j]:testOff[j+1]]

		n, err := w.Write(msg)
		if err != nil {
			b.Fatalf("write: %v", err)
		}
		if n != len(msg) {
			b.Fatalf("write %v of %v", n, len(msg))
		}

		written += n
	}

	//	b.Logf("total written: %x  %x", w.pos, w.pos/len(w.block))

	b.ReportMetric(float64(c.Bytes)/float64(written), "ratio")
	//	b.ReportMetric(float64(c.Operations)/float64(b.N), "writes/op")
	b.SetBytes(int64(written / b.N))
}

func BenchmarkDecodeFile(b *testing.B) {
	tl = nil
	//	tl = tlog.NewTestLogger(b, "", os.Stderr)

	err := loadTestFile(b, "../log.tlog")
	if err != nil {
		b.Skipf("loading data: %v", err)
	}

	var encoded low.Buf
	w := newEncoder(&encoded, 1024*1024, 6)

	written := 0
	for i := 0; i < 10000; i++ {
		j := i % testsCount
		msg := testData[testOff[j]:testOff[j+1]]

		n, err := w.Write(msg)
		if err != nil {
			b.Fatalf("write: %v", err)
		}
		if n != len(msg) {
			b.Fatalf("write %v of %v", n, len(msg))
		}

		written += n
	}

	b.ReportAllocs()
	b.ResetTimer()

	//	var decoded []byte
	var decoded bytes.Buffer
	r := NewDecoderBytes(encoded)

	for i := 0; i < b.N; i++ {
		r.ResetBytes(encoded)
		decoded.Reset()

		//	decoded, err = ioutil.ReadAll(r)
		_, err = decoded.ReadFrom(r)
		assert.NoError(b, err)
	}

	//	b.Logf("decoded %x", len(decoded))

	b.SetBytes(int64(decoded.Len()))

	min := len(testData)
	if min > decoded.Len() {
		min = decoded.Len()
	}
	assert.Equal(b, testData[:min], decoded.Bytes())

}

func loadTestFile(b testing.TB, f string) (err error) {
	if testData != nil {
		return
	}

	testData, err = ioutil.ReadFile(f)
	if err != nil {
		return errors.Wrap(err, "open data file")
	}

	d := tlog.NewDecoderBytes(testData)
	testOff = make([]int, 0, 100)

	st := 0
	for st < len(testData) {
		testOff = append(testOff, st)
		st = d.Skip(st)
	}
	testsCount = len(testOff)
	testOff = append(testOff, st)

	b.Logf("messages loaded: %v", testsCount)

	return
}
