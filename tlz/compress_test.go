package tlz

import (
	"bytes"
	"encoding/hex"
	"flag"
	"io"
	"io/ioutil"
	"testing"

	"github.com/nikandfor/assert"
	"github.com/nikandfor/errors"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/low"
	"github.com/nikandfor/tlog/tlio"
	"github.com/nikandfor/tlog/tlwire"
)

var fileFlag = flag.String("test-file", "../log.tlog", "file with tlog logs")

var (
	testData   []byte
	testOff    []int
	testsCount int
)

func TestLiteral(t *testing.T) {
	const B = 32

	var buf low.Buf

	w := newEncoder(&buf, B, 1)

	n, err := w.Write([]byte("very_first_message"))
	assert.Equal(t, 18, n)
	assert.NoError(t, err)

	t.Logf("buf pos %x ht %x\n%v", w.pos, w.ht, hex.Dump(w.block))
	t.Logf("res\n%v", hex.Dump(buf))

	r := &Decoder{
		b: buf,
	}

	p := make([]byte, 100)

	t.Logf("*** read back ***")

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

	var buf low.Buf

	w := newEncoder(&buf, B, 1)

	st := 0

	n, err := w.Write([]byte("prefix_1234_suffix"))
	assert.Equal(t, 18, n)
	assert.NoError(t, err)

	t.Logf("buf pos %x ht %x\n%v", w.pos, w.ht, hex.Dump(w.block))
	t.Logf("res\n%v", hex.Dump(buf[st:]))

	st = len(buf)

	n, err = w.Write([]byte("prefix_567_suffix"))
	assert.Equal(t, 17, n)
	assert.NoError(t, err)

	t.Logf("buf  pos %x ht %x\n%v", w.pos, w.ht, hex.Dump(w.block))
	t.Logf("res\n%v", hex.Dump(buf[st:]))

	r := &Decoder{
		b: buf,
	}

	p := make([]byte, 100)

	t.Logf("*** read back ***")

	n, err = r.Read(p[:10])
	assert.Equal(t, 10, n)
	assert.NoError(t, err)
	assert.Equal(t, []byte("prefix_123"), p[:n])

	t.Logf("buf  pos %x\n%v", r.pos, hex.Dump(r.block))

	n, err = r.Read(p[:10])
	assert.Equal(t, 10, n)
	assert.NoError(t, err)
	assert.Equal(t, []byte("4_suffixpr"), p[:n])

	t.Logf("buf  pos %x\n%v", r.pos, hex.Dump(r.block))

	n, err = r.Read(p[:30])
	assert.Equal(t, 15, n)
	assert.Equal(t, io.EOF, err)
	assert.Equal(t, []byte("efix_567_suffix"), p[:n])

	t.Logf("buf  pos %x\n%v", r.pos, hex.Dump(r.block))

	t.Logf("compression ratio: %.3f", float64(18+17)/float64(len(buf)))
}

func TestDumpOnelineText(t *testing.T) {
	t.Skip()

	var dump, text low.Buf

	d := NewDumper(&dump)
	e := newEncoder(d, 1*1024, 2)

	cw := tlog.NewConsoleWriter(tlio.NewTeeWriter(e, &text), tlog.LstdFlags)

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

func TestBug1(t *testing.T) {
	//	tl = tlog.NewTestLogger(t, "", nil)
	//	tlog.DefaultLogger = tl

	var b bytes.Buffer

	p := make([]byte, 1000)
	d := NewDecoder(&b)

	//	tl.Printw("first")

	_, _ = b.Write([]byte{Literal | Meta, MetaReset, 4})
	_, _ = b.Write([]byte{Literal | 3, 0x94, 0xa8, 0xfb, Copy | 9})

	n, err := d.Read(p)
	assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
	assert.Equal(t, 3, n)

	//	tl.Printw("second")

	_, _ = b.Write([]byte{0xfd, 0x03, 0x65}) // offset

	n, err = d.Read(p)
	assert.ErrorIs(t, err, io.EOF)
	assert.Equal(t, 9, n)
}

func BenchmarkLogCompressOneline(b *testing.B) {
	b.ReportAllocs()

	var buf low.Buf
	w := NewEncoder(&buf, 128*1024)
	var c tlio.CountableIODiscard

	l := tlog.New(io.MultiWriter(w, &c))
	tr := l.Start("span_name")

	types := []string{"type_a", "value_b", "qweqew", "asdads"}

	for i := 0; i < b.N; i++ {
		//	tr := l.Start("span_name")
		tr.Printw("some example message", "i", i, "type", types[i%len(types)])
		//	tr.Finish()
	}

	b.SetBytes(c.Bytes / int64(b.N))

	b.ReportMetric(float64(c.Bytes)/float64(len(buf)), "ratio")
}

func BenchmarkLogCompressOnelineText(b *testing.B) {
	b.ReportAllocs()

	var buf low.Buf
	w := NewEncoder(&buf, 128*1024)
	var c tlio.CountableIODiscard

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

	b.ReportMetric(float64(c.Bytes)/float64(len(buf)), "ratio")
}

func BenchmarkEncodeFile(b *testing.B) {
	err := loadTestFile(b, *fileFlag)
	if err != nil {
		b.Skipf("loading data: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	var c tlio.CountableIODiscard
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

	b.ReportMetric(float64(written)/float64(c.Bytes), "ratio")
	//	b.ReportMetric(float64(c.Operations)/float64(b.N), "writes/op")
	b.SetBytes(int64(written / b.N))
}

func BenchmarkDecodeFile(b *testing.B) {
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

func loadTestFile(tb testing.TB, f string) (err error) {
	tb.Helper()

	if testData != nil {
		return
	}

	testData, err = ioutil.ReadFile(f)
	if err != nil {
		return errors.Wrap(err, "open data file")
	}

	var d tlwire.Decoder
	testOff = make([]int, 0, 100)

	var st int
	for st < len(testData) {
		testOff = append(testOff, st)
		st = d.Skip(testData, st)
	}
	testsCount = len(testOff)
	testOff = append(testOff, st)

	tb.Logf("messages loaded: %v", testsCount)

	return
}
