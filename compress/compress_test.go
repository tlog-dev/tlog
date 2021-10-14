package compress

import (
	"bytes"
	"encoding/hex"
	"flag"
	"io"
	"io/ioutil"
	"os"
	"testing"

	"github.com/nikandfor/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/low"
	"github.com/nikandfor/tlog/tlio"
	"github.com/nikandfor/tlog/wire"
)

var fileFlag = flag.String("test-file", "../log.tlog", "file with tlog logs")

var (
	testData   []byte
	testOff    []int
	testsCount int
)

func TestLiteral(t *testing.T) {
	const B = 32

	tl = tlog.NewTestLogger(t, "", nil)

	// tl.Writer = tlio.NewTeeWriter(tl.Writer, tlog.NewDumper(os.Stderr))

	var buf low.Buf

	w := newEncoder(&buf, B, 1)

	n, err := w.Write([]byte("very_first_message"))
	assert.Equal(t, 18, n)
	assert.NoError(t, err)

	tl.Printf("buf pos %x ht %x\n%v", w.pos, w.ht, hex.Dump(w.block))
	tl.Printf("res\n%v", hex.Dump(buf))

	r := &Decoder{
		b: buf,
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

	// tl.Writer = tlio.NewTeeWriter(tl.Writer, tlog.NewDumper(os.Stderr))

	var buf low.Buf

	w := newEncoder(&buf, B, 1)

	st := 0

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

	r := &Decoder{
		b: buf,
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

	tl.Printw("compression", "ratio", float64(18+17)/float64(len(buf)))
}

func TestDumpOnelineText(t *testing.T) {
	t.Skip()

	tl = tlog.NewTestLogger(t, "", nil)

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
	tl = tlog.NewTestLogger(t, "", nil)
	tlog.DefaultLogger = tl

	var b bytes.Buffer

	p := make([]byte, 1000)
	d := NewDecoder(&b)

	tl.Printw("first")

	_, _ = b.Write([]byte{Literal | Meta, MetaReset, 10})
	_, _ = b.Write([]byte{Literal | 3, 0x94, 0xa8, 0xfb, Copy | 9})

	n, err := d.Read(p)
	assert.EqualError(t, err, io.ErrUnexpectedEOF.Error())
	assert.Equal(t, 3, n)

	tl.Printw("second")

	_, _ = b.Write([]byte{0xfd, 0x03, 0x65}) // offset

	n, err = d.Read(p)
	assert.EqualError(t, err, io.EOF.Error())
	assert.Equal(t, 9, n)
}

func TestDumpFile(t *testing.T) {
	t.Skip()

	const MaxEvents = 800

	f, err := os.Create("/tmp/seen.log") //nolint:gosec
	require.NoError(t, err)

	tl = nil
	tl = tlog.NewTestLogger(t, "hash",
		tlio.NewTeeWriter(
			//	tlog.Stderr,
			//	nil,
			f,
		),
	)

	data, err := ioutil.ReadFile(*fileFlag)
	if err != nil {
		t.Skipf("open test data: %v", err)
	}

	var dec wire.Decoder
	var dump, encoded low.Buf

	d := NewDumper(&dump)
	w := newEncoder(tlio.NewTeeWriter(&encoded, d), 16*1024, 6)

	var st int
	for n := 0; n < MaxEvents && st < len(data); n++ {
		end := dec.Skip(data, st)

		//	tl.Printw("write event", "st", tlog.Hex(st), "end", tlog.Hex(end))

		m, err := w.Write(data[st:end])
		if !assert.NoError(t, err, "%v bytes written", m) {
			break
		}

		st = end
	}

	//	t.Logf("dump:\n%s", dump)
	err = ioutil.WriteFile("/tmp/seen.dump", dump, 0600) //nolint:gosec
	require.NoError(t, err)
	t.Logf("block size: %x", len(w.block))
	t.Logf("writer pos: %x", w.pos)

	r := NewDecoderBytes(encoded)

	decoded, err := ioutil.ReadAll(r)
	assert.NoError(t, err)

	assert.Equal(t, data[:len(decoded)], decoded)
}

func BenchmarkLogCompressOneline(b *testing.B) {
	tl = nil

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
	tl = nil

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
	tl = nil
	//	tl = tlog.NewTestLogger(b, "", os.Stderr)

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

func loadTestFile(tb testing.TB, f string) (err error) {
	tb.Helper()

	if testData != nil {
		return
	}

	testData, err = ioutil.ReadFile(f)
	if err != nil {
		return errors.Wrap(err, "open data file")
	}

	var d wire.Decoder
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
