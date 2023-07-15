package tlz

import (
	"bytes"
	"encoding/hex"
	"flag"
	"io"
	"io/ioutil"
	"testing"

	//"github.com/nikandfor/assert"
	"github.com/nikandfor/errors"
	"github.com/stretchr/testify/assert"

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

func TestFileMagic(t *testing.T) {
	var buf low.Buf

	w := NewEncoder(&buf, MiB)

	_, err := w.Write([]byte{})
	assert.NoError(t, err)

	if assert.True(t, len(buf) >= len(FileMagic)) {
		assert.Equal(t, FileMagic, string(buf[:len(FileMagic)]))
	}
}

func TestLiteral(t *testing.T) {
	const B = 32

	var buf low.Buf

	w := newEncoder(&buf, B, 1)

	n, err := w.Write([]byte("very_first_message"))
	assert.Equal(t, 18, n)
	assert.NoError(t, err)

	t.Logf("buf pos %x ht %x\n%v", w.pos, w.ht, hex.Dump(w.block))
	t.Logf("res\n%v", hex.Dump(buf))
	t.Logf("res\n%v", Dump(buf))

	r := &Decoder{
		b: buf,
	}

	p := make([]byte, 100)

	t.Logf("*** read back ***")

	n, err = r.Read(p[:10])
	assert.Equal(t, 10, n)
	assert.NoError(t, err)
	assert.Equal(t, []byte("very_first"), p[:n])

	copy(p[:10], zeros)

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

	//	t.Logf("compression ratio: %.3f", float64(18+17)/float64(len(buf)))
}

func TestDumpOnelineText(t *testing.T) {
	t.Skip()

	var dump, text low.Buf

	d := NewDumper(&dump)
	e := newEncoder(d, 1*1024, 2)

	cw := tlog.NewConsoleWriter(tlio.NewMultiWriter(e, &text), tlog.LstdFlags)

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

	_, _ = b.Write([]byte{Literal | Meta, MetaReset | 0, 4})
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

func TestOnFile(t *testing.T) {
	err := loadTestFile(t, *fileFlag)
	if err != nil {
		t.Skipf("loading data: %v", err)
	}

	var encoded bytes.Buffer
	var full bytes.Buffer
	w := NewEncoderHTSize(tlio.NewMultiWriter(&encoded, &full), 512, 256)
	r := NewDecoder(&encoded)
	var buf []byte

	//	dumper := tlwire.NewDumper(os.Stderr)

	for i := 0; i < testsCount; i++ {
		msg := testData[testOff[i]:testOff[i+1]]

		//	_, _ = dumper.Write(msg)

		n, err := w.Write(msg)
		assert.NoError(t, err)
		assert.Equal(t, len(msg), n)

		for n > len(buf) {
			buf = append(buf[:cap(buf)], 0, 0, 0, 0, 0, 0, 0, 0)
		}

		n, err = r.Read(buf[:n])
		assert.NoError(t, err)
		assert.Equal(t, len(msg), n)

		assert.Equal(t, msg, []byte(buf[:n]))

		if t.Failed() {
			break
		}
	}

	r.Reset(&full)
	buf = buf[:0]

	var dec bytes.Buffer

	n, err := io.Copy(&dec, r)
	assert.NoError(t, err)
	assert.Equal(t, int(n), dec.Len())

	min := dec.Len()
	assert.Equal(t, testData[:min], dec.Bytes())

	//	t.Logf("metrics: %v  bytes %v  events %v", mm, dec.Len(), testsCount)
}

func BenchmarkLogCompressOneline(b *testing.B) {
	b.ReportAllocs()

	var full, small tlio.CountingIODiscard
	w := NewEncoder(&small, 128*1024)

	l := tlog.New(io.MultiWriter(&full, w))
	tr := l.Start("span_name")

	types := []string{"type_a", "value_b", "qweqew", "asdads"}

	for i := 0; i < b.N; i++ {
		//	tr := l.Start("span_name")
		tr.Printw("some example message", "i", i, "type", types[i%len(types)])
		//	tr.Finish()
	}

	b.SetBytes(full.Bytes.Load() / int64(b.N))
	b.ReportMetric(float64(full.Bytes.Load())/float64(small.Bytes.Load()), "ratio")
}

func BenchmarkLogCompressOnelineText(b *testing.B) {
	b.ReportAllocs()

	var full, small tlio.CountingIODiscard
	w := NewEncoder(&small, 128*1024)
	cw := tlog.NewConsoleWriter(io.MultiWriter(&full, w), tlog.LstdFlags)

	l := tlog.New(cw)
	tr := l.Start("span_name")

	types := []string{"type_a", "value_b", "qweqew", "asdads"}

	for i := 0; i < b.N; i++ {
		//	tr := l.Start("span_name")
		tr.Printw("some example message", "i", i, "type", types[i%len(types)])
		//	tr.Finish()
	}

	b.SetBytes(full.Bytes.Load() / int64(b.N))
	b.ReportMetric(float64(full.Bytes.Load())/float64(small.Bytes.Load()), "ratio")
}

const BlockSize, HTSize = 1024 * 1024, 16 * 1024

func BenchmarkEncodeFile(b *testing.B) {
	err := loadTestFile(b, *fileFlag)
	if err != nil {
		b.Skipf("loading data: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	var c tlio.CountingIODiscard
	w := NewEncoderHTSize(&c, BlockSize, HTSize)

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

	b.ReportMetric(float64(written)/float64(c.Bytes.Load()), "ratio")
	//	b.ReportMetric(float64(c.Operations)/float64(b.N), "writes/op")
	b.SetBytes(int64(written / b.N))
}

func BenchmarkDecodeFile(b *testing.B) {
	err := loadTestFile(b, *fileFlag)
	if err != nil {
		b.Skipf("loading data: %v", err)
	}

	encoded := make(low.Buf, 0, len(testData)/2)
	w := NewEncoderHTSize(&encoded, BlockSize, HTSize)

	const limit = 20000

	written := 0
	for i := 0; i < testsCount && i < limit; i++ {
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

	b.ReportMetric(float64(written)/float64(len(encoded)), "ratio")

	//	var decoded []byte
	decoded := make(low.Buf, 0, len(testData))
	buf := make([]byte, 4096)
	r := NewDecoderBytes(encoded)

	for i := 0; i < b.N/testsCount; i++ {
		r.ResetBytes(encoded)
		decoded = decoded[:0]

		_, err = io.CopyBuffer(&decoded, r, buf)
		assert.NoError(b, err)
	}

	//	b.Logf("decoded %x", len(decoded))

	b.SetBytes(int64(decoded.Len() / testsCount))

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
	testOff = make([]int, 0, len(testData)/100)

	var st int
	for st < len(testData) {
		testOff = append(testOff, st)
		st = d.Skip(testData, st)
	}
	testsCount = len(testOff)
	testOff = append(testOff, st)

	tb.Logf("events loaded: %v", testsCount)

	return
}

func FuzzEncoder(f *testing.F) {
	f.Add(
		[]byte("prefix_1234_suffix"),
		[]byte("prefix_567_suffix"),
		[]byte("suffix_prefix"),
	)

	f.Add(
		[]byte("aaaaaa"),
		[]byte("aaaaaaaaaaaa"),
		[]byte("aaaaaaaaaaaaaaaaaaaaaaaa"),
	)

	f.Add(
		[]byte("aaaaab"),
		[]byte("aaaaabaaaaaa"),
		[]byte("aaaaaaaaaaabaaaaaaaaaaaa"),
	)

	var ebuf, dbuf bytes.Buffer
	buf := make([]byte, 16)

	e := NewEncoderHTSize(&ebuf, 512, 32)
	d := NewDecoder(&dbuf)

	f.Fuzz(func(t *testing.T, p0, p1, p2 []byte) {
		e.Reset(e.Writer)
		ebuf.Reset()

		for _, p := range [][]byte{p0, p1, p2} {
			n, err := e.Write(p)
			assert.NoError(t, err)
			assert.Equal(t, len(p), n)
		}

		d.ResetBytes(ebuf.Bytes())
		dbuf.Reset()

		m, err := io.CopyBuffer(&dbuf, d, buf)
		assert.NoError(t, err)
		assert.Equal(t, len(p0)+len(p1)+len(p2), int(m))

		i := 0
		for _, p := range [][]byte{p0, p1, p2} {
			assert.Equal(t, p, dbuf.Bytes()[i:i+len(p)])
			i += len(p)
		}

		assert.Equal(t, int(m), i)

		if !t.Failed() {
			return
		}

		for i, p := range [][]byte{p0, p1, p2} {
			t.Logf("p%d\n%s", i, hex.Dump(p))
		}

		t.Logf("encoded dump\n%s", Dump(ebuf.Bytes()))
	})
}
