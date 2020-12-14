package compress

import (
	"encoding/hex"
	"io"
	"io/ioutil"
	"testing"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/low"
	"github.com/stretchr/testify/assert"
)

func TestCompare(t *testing.T) {
	const B = 32

	w := newEncoder(nil, B, 0)

	st, end := w.compare([]byte("1234567890"), 0, 0)
	assert.Equal(t, 0, st)
	assert.Equal(t, 0, end)

	copy(w.block, "12345678901234567890")
	w.pos = len(w.block) + 2

	t.Logf("block: %d %q", len(w.block), w.block)

	st, end = w.compare([]byte("1234567890"), 0, 0)
	assert.Equal(t, 0, st)
	assert.Equal(t, 10, end)

	st, end = w.compare([]byte("123456789012"), 2, 2)
	assert.Equal(t, 0, st)
	assert.Equal(t, 12, end)

	copy(w.block, "4567890             ")
	copy(w.block[B-3:], "123")

	st, end = w.compare([]byte("1234567890"), 1, B-2)
	assert.Equal(t, B-3, st)
	assert.Equal(t, B+7, end)

	copy(w.block, "890             ")
	copy(w.block[B-len("bcdef1234567"):], "bcdef1234567")

	st, end = w.compare([]byte("++++abcdef1234567890qw"), len("++++abcdef12345678"), B+1)
	assert.Equal(t, B-len("bcdef1234567"), st)
	assert.Equal(t, B+3, end)
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
		end: len(buf),
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
		end: len(buf),
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
	t.Skip()

	tl = nil

	data, err := ioutil.ReadFile("../log.tlog")
	if err != nil {
		t.Skipf("open test data: %v", err)
	}

	dec := tlog.NewDecoderBytes(data)

	var dump low.Buf

	d := NewDumper(&dump)
	w := NewEncoder(d, 128*1024)

	st := 0
	for n := 0; n < 40 && st < len(data); n++ {
		end := dec.Skip(st)

		m, err := w.Write(data[st:end])
		if !assert.NoError(t, err, "%v bytes written", m) {
			break
		}

		st = end
	}

	t.Logf("dump:\n%s", dump)
}

func TestDumpOnelineText(t *testing.T) {
	tl = tlog.NewTestLogger(t, "", nil)

	var dump, text low.Buf

	d := NewDumper(&dump)
	e := NewEncoder(d, 128*1024)

	cw := tlog.NewConsoleWriter(tlog.NewTeeWriter(e, &text), tlog.LstdFlags)

	l := tlog.New(cw)
	tr := l.Start("span_name")

	types := []string{"type_a", "value_b", "qweqew", "asdads"}

	for i := 0; i < 10; i++ {
		//	tr := l.Start("span_name")
		tr.Printw("some example message", "i", i, "type", types[i%len(types)])
		//	tr.Finish()
	}

	t.Logf("text:\n%s", text)
	t.Logf("dump:\n%s", dump)
}

func BenchmarkCompressOneline(b *testing.B) {
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

func BenchmarkCompressOnelineText(b *testing.B) {
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

func BenchmarkCompressFile(b *testing.B) {
	tl = nil

	b.ReportAllocs()

	data, err := ioutil.ReadFile("../log.tlog")
	if err != nil {
		b.Skipf("open test data: %v", err)
	}

	d := tlog.NewDecoderBytes(data)
	off := make([]int, 0, 100)

	st := 0
	for st < len(data) {
		off = append(off, st)
		st = d.Skip(st)
	}
	n := len(off)
	off = append(off, st)

	b.Logf("messages loaded: %v", n)

	b.ResetTimer()

	var c tlog.CountableIODiscard
	w := newEncoder(&c, 1024*1024, 0)

	written := 0

	for i := 0; i < b.N; i++ {
		j := i % n
		msg := data[off[j]:off[j+1]]

		n, err := w.Write(msg)
		if err != nil {
			b.Fatalf("write: %v", err)
		}
		if n != len(msg) {
			b.Fatalf("write %v of %v", n, len(msg))
		}

		written += n
	}

	b.ReportMetric(float64(c.Bytes)/float64(written), "ration")
	b.SetBytes(int64(written / b.N))
}
