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

	"tlog.app/go/errors"

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
		BlockSize int64

		Stderr io.Writer

		d tlwire.Decoder
	}

	stream struct {
		labels []byte
		sum    uint32

		file *file
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

		Partition:    3 * time.Hour,
		KeyTimestamp: tlog.KeyTimestamp,

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
	part := time.Unix(0, ts).Truncate(a.Partition).UnixNano()

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
	}

	if s.file == nil {
		f, err := a.newFile(s, part, ts)
		if err != nil {
			return nil, nil, errors.Wrap(err, "new file")
		}

		s.file = f
	}

	return s.file, s, nil
}

func (a *Agent) newFile(s *stream, part, ts int64) (*file, error) {
	base := fmt.Sprintf("%08x/%08x_%08x.tlz", part/1e9, s.sum, ts/1e9)
	fname := filepath.Join(a.path, base)
	dir := filepath.Dir(fname)

	err := os.MkdirAll(dir, 0o755)
	if err != nil {
		return nil, errors.Wrap(err, "mkdir")
	}

	w, err := os.OpenFile(fname, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, errors.Wrap(err, "open file")
	}

	f := &file{
		w: w,

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

func (a *Agent) writeFile(s *stream, f *file, p []byte, ts int64) (n int, err error) {
	defer f.mu.Unlock()
	f.mu.Lock()

	st := f.off

	n, err = f.w.Write(p)
	if err != nil {
		return
	}

	f.off += int64(n)

	if len(f.index) == 0 || f.off >= f.index[len(f.index)-1].off+a.BlockSize {
		f.index = append(f.index, ientry{
			off: st,
			ts:  ts,
		})
	}

	return
}
