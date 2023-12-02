package agent

import (
	"bytes"
	stderrors "errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	hlow "github.com/nikandfor/hacked/low"
	"tlog.app/go/eazy"
	"tlog.app/go/errors"
	"tlog.app/go/loc"

	"tlog.app/go/tlog"
	"tlog.app/go/tlog/tlwire"
)

type (
	Agent struct {
		path string

		mu sync.Mutex

		subid int64 // last used
		subs  []sub

		streams []*stream
		files   []*file

		// end of mu

		KeyTimestamp string

		Partition time.Duration
		FileSize  int64
		BlockSize int64

		Stderr io.Writer

		d tlwire.Decoder
	}

	stream struct {
		labels []byte
		sum    uint32

		file *file

		z    *eazy.Writer
		zbuf hlow.Buf
		boff int64
	}

	file struct {
		w io.Writer

		name string

		part int64
		ts   int64

		prev *file

		mu sync.Mutex

		off   int64
		index []ientry
	}

	ientry struct {
		off int64
		ts  int64
	}
)

var (
	ErrUnknownSubscription = stderrors.New("unknown subscription")

	ErrFileFull = stderrors.New("file is full")
)

func New(path string) (*Agent, error) {
	a := &Agent{
		path: path,

		KeyTimestamp: tlog.KeyTimestamp,
		Partition:    3 * time.Hour,
		FileSize:     eazy.GiB,
		BlockSize:    16 * eazy.MiB,

		Stderr: os.Stderr,
	}

	return a, nil
}

func (a *Agent) Write(p []byte) (n int, err error) {
	defer a.mu.Unlock()
	a.mu.Lock()

	for n < len(p) {
		ts, labels, err := a.parseEventHeader(p[n:])
		if err != nil {
			return n, errors.Wrap(err, "parse event")
		}

		f, s, err := a.file(ts, labels, len(p[n:]))
		if err != nil {
			return n, errors.Wrap(err, "get file")
		}

		m, err := a.writeFile(s, f, p[n:], ts)
		n += m
		if errors.Is(err, ErrFileFull) {
			continue
		}
		if err != nil {
			return n, errors.Wrap(err, "write")
		}
	}

	return
}

func (a *Agent) parseEventHeader(p []byte) (ts int64, labels []byte, err error) {
	tag, els, i := a.d.Tag(p, 0)
	if tag != tlwire.Map {
		err = errors.New("expected map")
		return
	}

	var k []byte
	var sub int64
	var end int

	for el := 0; els == -1 || el < int(els); el++ {
		if els == -1 && a.d.Break(p, &i) {
			break
		}

		st := i

		k, i = a.d.Bytes(p, i)
		if len(k) == 0 {
			err = errors.New("empty key")
			return
		}

		tag, sub, end = a.d.SkipTag(p, i)
		if tag != tlwire.Semantic {
			i = a.d.Skip(p, i)
			continue
		}

		switch {
		case sub == tlwire.Time && string(k) == a.KeyTimestamp:
			ts, i = a.d.Timestamp(p, i)
		case sub == tlog.WireLabel:
			// labels = crc32.Update(labels, crc32.IEEETable, p[st:end])
			labels = append(labels, p[st:end]...)
		}

		i = end
	}

	return
}

func (a *Agent) file(ts int64, labels []byte, size int) (*file, *stream, error) {
	sum := crc32.ChecksumIEEE(labels)

	var s *stream

	for _, ss := range a.streams {
		if ss.sum == sum && bytes.Equal(ss.labels, labels) {
			s = ss
			break
		}
	}

	if s == nil {
		s = &stream{
			labels: labels,
			sum:    sum,
		}

		s.z = eazy.NewWriter(&s.zbuf, eazy.MiB, 1024)
		s.z.AppendMagic = true

		a.streams = append(a.streams, s)
	}

	return s.file, s, nil
}

func (a *Agent) writeFile(s *stream, f *file, p []byte, ts int64) (n int, err error) {
	tlog.Printw("write message", "i", geti(p))

	s.zbuf = s.zbuf[:0]
	n, err = s.z.Write(p)
	if err != nil {
		return 0, errors.Wrap(err, "eazy")
	}

	part := time.Unix(0, ts).Truncate(a.Partition).UnixNano()

	if f == nil || f.part != part || f.off+int64(len(s.zbuf)) > a.FileSize && f.off != 0 {
		f, err = a.newFile(s, part, ts)
		if err != nil {
			return 0, errors.Wrap(err, "new file")
		}

		s.file = f

		s.z.Reset(&s.zbuf)
		s.boff = 0
	}

	defer f.mu.Unlock()
	f.mu.Lock()

	//	tlog.Printw("write file", "file", f.name, "off", tlog.NextAsHex, f.off, "boff", tlog.NextAsHex, s.boff, "block", tlog.NextAsHex, a.BlockSize)
	nextBlock := false

	if nextBlock := s.boff+int64(len(s.zbuf)) > a.BlockSize && s.boff != 0; nextBlock {
		err = a.padFile(s, f)
		tlog.Printw("pad file", "off", tlog.NextAsHex, f.off, "err", err)
		if err != nil {
			return 0, errors.Wrap(err, "pad file")
		}

		s.z.Reset(&s.zbuf)
		s.boff = 0
	}

	if len(f.index) == 0 || nextBlock {
		tlog.Printw("append index", "off", tlog.NextAsHex, f.off, "ts", ts/1e9)
		f.index = append(f.index, ientry{
			off: f.off,
			ts:  ts,
		})
	}

	if s.boff == 0 {
		s.zbuf = s.zbuf[:0]
		n, err = s.z.Write(p)
		if err != nil {
			return 0, errors.Wrap(err, "eazy")
		}
	}

	n, err = f.w.Write(s.zbuf)
	//	tlog.Printw("write message", "zst", tlog.NextAsHex, zst, "n", tlog.NextAsHex, n, "err", err)
	if err != nil {
		return n, err
	}

	f.off += int64(n)
	s.boff += int64(n)

	return len(p), nil
}

func (a *Agent) newFile(s *stream, part, ts int64) (*file, error) {
	//	base := fmt.Sprintf("%08x/%08x_%08x.tlz", part/1e9, s.sum, ts/1e9)
	base := fmt.Sprintf("%v/%08x_%08x.tlz",
		time.Unix(0, part).UTC().Format("2006-01-02T15:04"),
		s.sum,
		ts/1e9,
	)
	fname := filepath.Join(a.path, base)
	dir := filepath.Dir(fname)

	tlog.Printw("new file", "file", base, "from", loc.Callers(1, 2))

	err := os.MkdirAll(dir, 0o755)
	if err != nil {
		return nil, errors.Wrap(err, "mkdir")
	}

	w, err := os.OpenFile(fname, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, errors.Wrap(err, "open file")
	}

	f := &file{
		w:    w,
		name: fname,

		prev: s.file,

		part: part,
		ts:   ts,

		//	index: []ientry{{
		//		off: 0,
		//		ts:  ts,
		//	}},
	}

	return f, nil
}

func (a *Agent) padFile(s *stream, f *file) error {
	if f.off%int64(a.BlockSize) == 0 {
		s.boff = 0

		return nil
	}

	off := f.off + int64(a.BlockSize) - f.off%int64(a.BlockSize)

	if s, ok := f.w.(interface {
		Truncate(int64) error
		io.Seeker
	}); ok {
		err := s.Truncate(off)
		if err != nil {
			return errors.Wrap(err, "truncate")
		}

		off, err = s.Seek(off, io.SeekStart)
		if err != nil {
			return errors.Wrap(err, "seek")
		}

		f.off = off
	} else {
		n, err := f.w.Write(make([]byte, off-f.off))
		if err != nil {
			return errors.Wrap(err, "write padding")
		}

		f.off += int64(n)
	}

	return nil
}

func geti(p []byte) (x int64) {
	var d tlwire.LowDecoder

	tag, els, i := d.Tag(p, 0)
	if tag != tlwire.Map {
		return -1
	}

	var k []byte
	var sub int64
	var end int

	for el := 0; els == -1 || el < int(els); el++ {
		if els == -1 && d.Break(p, &i) {
			break
		}

		k, i = d.Bytes(p, i)
		if len(k) == 0 {
			return -1
		}

		tag, sub, end = d.SkipTag(p, i)
		if tag == tlwire.Int && string(k) == "i" {
			return sub
		}

		i = end
	}

	return -1
}
