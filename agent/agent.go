package agent

import (
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"sync"
	"time"

	"github.com/nikandfor/errors"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/tlio"
	"github.com/nikandfor/tlog/tlwire"
	"github.com/nikandfor/tlog/tlz"
)

type (
	Agent struct {
		path string

		sync.Mutex

		partitions []*partition // recent to oldest

		// end of Mutex

		Partition    time.Duration
		KeyTimestamp string

		Stderr io.Writer

		d tlwire.Decoder
	}

	partition struct {
		*Agent

		Start time.Time

		streams map[uint32]*Stream
	}

	Stream struct {
		io.Writer
		sync.Mutex

		part *partition
		sum  uint32
	}
)

func New(db string) (*Agent, error) {
	a := &Agent{
		path: db,

		Partition:    24 * time.Hour,
		KeyTimestamp: tlog.KeyTimestamp,

		Stderr: os.Stderr,
	}

	return a, nil
}

func (w *Agent) Write(p []byte) (i int, err error) {
	defer func() {
		perr := recover()

		if err == nil && perr == nil {
			return
		}

		if perr != nil {
			fmt.Fprintf(w.Stderr, "panic: %v (pos %x)\n", perr, i)
		} else {
			fmt.Fprintf(w.Stderr, "parse error: %+v (pos %x)\n", err, i)
		}
		fmt.Fprintf(w.Stderr, "dump\n%v", tlwire.Dump(p))
		fmt.Fprintf(w.Stderr, "hex dump\n%v", hex.Dump(p))

		s := debug.Stack()
		fmt.Fprintf(w.Stderr, "%s", s)
	}()

	tlog.V("dump").Write(p)

	ts, labels, err := w.parseEventHeader(p)
	if err != nil {
		return 0, errors.Wrap(err, "parse event header")
	}

	start := time.Unix(0, ts).Truncate(w.Partition)

	w.Lock()

	part := w.getPartition(start)
	stream := part.getStream(labels)

	w.Unlock()

	return stream.Write(p)
}

func (w *Agent) parseEventHeader(p []byte) (ts int64, labels uint32, err error) {
	tag, els, i := w.d.Tag(p, 0)
	if tag != tlwire.Map {
		err = errors.New("expected map")
		return
	}

	var k []byte
	var sub int64
	var end int

	for el := 0; els == -1 || el < int(els); el++ {
		if els == -1 && w.d.Break(p, &i) {
			break
		}

		k, i = w.d.Bytes(p, i)
		if len(k) == 0 {
			err = errors.New("empty key")
			return
		}

		tag, sub, end = w.d.SkipTag(p, i)
		if tag != tlwire.Semantic {
			i = w.d.Skip(p, i)
			continue
		}

		switch {
		case sub == tlwire.Time && string(k) == w.KeyTimestamp:
			ts, i = w.d.Timestamp(p, i)
		case sub == tlog.WireLabel:
			labels = crc32.Update(labels, crc32.IEEETable, p[i:end])
		}

		i = end
	}

	return
}

func (w *Agent) Close() (err error) {
	w.Lock()
	defer w.Unlock()

	for _, p := range w.partitions {
		e := p.Close()
		if err == nil {
			err = errors.Wrap(e, "part %v", p.Start)
		}
	}

	return err
}

func (w *Agent) newPartition(start time.Time) *partition {
	return &partition{
		Agent:   w,
		Start:   start,
		streams: map[uint32]*Stream{},
	}
}

func (w *Agent) getPartition(start time.Time) *partition {
	for _, p := range w.partitions {
		if p.Start == start {
			return p
		}

		if p.Start.Before(start) {
			break
		}
	}

	p := w.newPartition(start)

	tlog.Printw("new partition", "start", start)

	w.partitions = append(w.partitions, p)

	sort.Slice(w.partitions, func(i, j int) bool {
		return w.partitions[i].Start.After(w.partitions[j].Start)
	})

	return p
}

func (p *partition) newStream(sum uint32) *Stream {
	return &Stream{
		part: p,
		sum:  sum,
	}
}

func (p *partition) getStream(sum uint32) *Stream {
	s, ok := p.streams[sum]
	if ok {
		return s
	}

	s = p.newStream(sum)
	p.streams[sum] = s

	tlog.Printw("new stream", "sum", tlog.NextIsHex, sum)

	return s
}

func (p *partition) Close() (err error) {
	for _, s := range p.streams {
		e := s.Close()
		if err == nil {
			err = errors.Wrap(e, "stream %08x", s.sum)
		}
	}

	return err
}

func (s *Stream) Write(p []byte) (i int, err error) {
	defer s.Unlock()
	s.Lock()

	if s.Writer == nil {
		s.Writer, err = s.openWriter()
		if err != nil {
			return 0, errors.Wrap(err, "open writer")
		}
	}

	i, err = s.Writer.Write(p)

	return
}

func (s *Stream) Close() (err error) {
	if c, ok := s.Writer.(io.Closer); ok {
		err = c.Close()
	}

	return err
}

func (s *Stream) openWriter() (io.Writer, error) {
	name := filepath.Join(s.part.Agent.path, s.part.Start.Format("2006-01-02_15:04"), fmt.Sprintf("%08x.tlz", s.sum))
	dir := filepath.Dir(name)

	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return nil, errors.Wrap(err, "mkdir")
	}

	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return nil, errors.Wrap(err, "open file")
	}

	w := tlz.NewEncoder(f, tlz.MiB)

	return tlio.WriteCloser{
		Writer: w,
		Closer: f,
	}, nil
}
