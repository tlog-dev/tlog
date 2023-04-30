package tlz

import (
	"bytes"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
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

func TestLiteral(t *testing.T) {
	const B = 32

	var buf low.Buf

	w := newEncoder(&buf, B, 1)

	n, err := w.Write([]byte("very_first_message"))
	assert.Equal(t, 18, n)
	assert.NoError(t, err)

	t.Logf("buf pos %x ht %x\n%v", w.pos, w.ht, hex.Dump(w.block))
	t.Logf("res\n%v", Dump(buf))
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

func TestOnFile(t *testing.T) {
	err := loadTestFile(t, *fileFlag)
	if err != nil {
		t.Skipf("loading data: %v", err)
	}

	var encoded low.Buf
	w := NewEncoderHTSize(&encoded, 4096, 256)

	tldumper := tlwire.NewDumper(os.Stderr)
	tlzdumper := NewDumper(os.Stderr)

	dumpN := 4

	written := 0
	for i := 0; i < 20 && i < testsCount; i++ {
		j := i % testsCount
		msg := testData[testOff[j]:testOff[j+1]]

		if i < dumpN {
			//	fmt.Fprintf(os.Stderr, "current block\n%s", hex.Dump(w.block))
			fmt.Fprintf(os.Stderr, "current ht\n%x\n", w.ht)
			fmt.Fprintf(os.Stderr, "w.pos %x  hmask %x\n", w.pos, w.hmask)
			fmt.Fprintf(os.Stderr, "message\n")
			tldumper.Write(msg)
		}
		ww := w.written

		n, err := w.Write(msg)
		if err != nil {
			t.Fatalf("write: %v", err)
		}
		if n != len(msg) {
			t.Fatalf("write %v of %v", n, len(msg))
		}

		if i < dumpN {
			fmt.Fprintf(os.Stderr, "compressed\n")
			tlzdumper.Write(encoded[ww:])
		}

		written += n
	}

	var decoded low.Buf
	r := NewDecoderBytes(encoded)

	n, err := io.Copy(&decoded, r)
	assert.NoError(t, err)
	assert.Equal(t, len(decoded), int(n))

	min := len(decoded)
	assert.Equal(t, testData[:min], decoded.Bytes())
}

func BenchmarkLogCompressOneline(b *testing.B) {
	b.ReportAllocs()

	var buf low.Buf
	w := NewEncoder(&buf, 128*1024)
	var c tlio.CountingIODiscard

	l := tlog.New(io.MultiWriter(w, &c))
	tr := l.Start("span_name")

	types := []string{"type_a", "value_b", "qweqew", "asdads"}

	for i := 0; i < b.N; i++ {
		//	tr := l.Start("span_name")
		tr.Printw("some example message", "i", i, "type", types[i%len(types)])
		//	tr.Finish()
	}

	b.SetBytes(c.Bytes.Load() / int64(b.N))

	b.ReportMetric(float64(c.Bytes.Load())/float64(len(buf)), "ratio")
}

func BenchmarkLogCompressOnelineText(b *testing.B) {
	b.ReportAllocs()

	var buf low.Buf
	w := NewEncoder(&buf, 128*1024)
	var c tlio.CountingIODiscard

	cw := tlog.NewConsoleWriter(io.MultiWriter(w, &c), tlog.LstdFlags)

	l := tlog.New(cw)
	tr := l.Start("span_name")

	types := []string{"type_a", "value_b", "qweqew", "asdads"}

	for i := 0; i < b.N; i++ {
		//	tr := l.Start("span_name")
		tr.Printw("some example message", "i", i, "type", types[i%len(types)])
		//	tr.Finish()
	}

	b.SetBytes(c.Bytes.Load() / int64(b.N))

	b.ReportMetric(float64(c.Bytes.Load())/float64(len(buf)), "ratio")
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

	written := 0
	for i := 0; i < testsCount && i < 10000; i++ {
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
