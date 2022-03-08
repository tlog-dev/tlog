package agent

import (
	"context"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"sync"

	"github.com/nikandfor/errors"
	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/tq/parse"
	"github.com/nikandfor/tlog/wire"
)

type (
	DB struct {
		fs fs.ReadDirFS

		log  *tlog.Logger
		logf io.Closer

		mu sync.Mutex

		s map[string]*StreamInfo
	}

	StreamInfo struct {
		ID     string
		Labels tlog.Labels

		s *Stream
	}
)

func NewDB(fs fs.ReadDirFS) (*DB, error) {
	d := &DB{
		fs: fs,

		s: make(map[string]*StreamInfo),
	}

	return d, nil
}

func (d *DB) Close() (err error) {
	defer d.mu.Unlock()
	d.mu.Lock()

	for _, s := range d.s {
		if s.s == nil {
			continue
		}

		e := s.s.Close()
		if err == nil {
			err = errors.Wrap(e, "close stream")
		}
	}

	return
}

func (d *DB) ReadStream(ctx context.Context, id string) (io.WriterTo, error) {
	s, ok := d.s[id]
	if !ok {
		return nil, nil
	}

	return s.s.OpenReader()
}

func (d *DB) allLabels() tlog.Labels {
	defer d.mu.Unlock()
	d.mu.Lock()

	m := map[string]struct{}{}

	for _, s := range d.s {
		for _, l := range s.Labels {
			m[l] = struct{}{}
		}
	}

	ls := make(tlog.Labels, len(m))

	i := 0
	for l := range m {
		ls[i] = l
		i++
	}

	return ls
}

func (d *DB) allStreams() []StreamInfo {
	defer d.mu.Unlock()
	d.mu.Lock()

	cp := make([]StreamInfo, len(d.s))

	i := 0
	for _, s := range d.s {
		cp[i] = *s
		cp[i].s = nil
		i++
	}

	return cp
}

func (d *DB) Write(p []byte) (n int, err error) {
	//	ctx := context.Background()
	//	defer func() {
	//		tlog.Printw("db.write", "n", n, "err", err, "from", loc.Caller(1))
	//	}()

	var ev parse.LowEvent

	n, err = ev.Parse(p, n)
	if err != nil {
		return 0, errors.Wrap(err, "parse event")
	}
	if n != len(p) {
		panic("multiple events")
	}

	if ev.Timestamp() == 0 {
		return 0, errors.New("no timestamp")
	}

	var dec wire.Decoder
	ls := ev.Labels()

	if len(ls) != 0 {
		i := 0
		for {
			j := dec.Skip(ls, i)

			if j == len(ls) {
				ls = ls[i:]
				break
			}

			i = j
		}
	}

	sum := crc32.ChecksumIEEE(ls)
	id := fmt.Sprintf("%08x", sum)

	defer d.mu.Unlock()
	d.mu.Lock()

	s, ok := d.s[id]
	if !ok {
		s = &StreamInfo{
			ID: id,
		}

		sub, err := fs.Sub(d.fs, id)
		if err != nil {
			return 0, errors.Wrap(err, "subfs")
		}

		s.s, err = NewStream(sub)
		if err != nil {
			return 0, errors.Wrap(err, "open stream")
		}

		_ = s.Labels.TlogParse(&dec, ls, 0)

		d.s[id] = s
	}

	err = s.s.WriteEvent(&ev)
	if err != nil {
		return 0, errors.Wrap(err, "write to stream")
	}

	return len(p), nil
}
