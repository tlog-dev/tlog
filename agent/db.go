package agent

import (
	"context"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nikandfor/errors"
	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/compress"
	"github.com/nikandfor/tlog/tlio"
	"github.com/nikandfor/tlog/tq/parse"
	"github.com/nikandfor/tlog/wire"
)

type (
	DB struct {
		dir string

		MaxFileSize   int64
		MaxFileEvents int

		mu sync.Mutex

		s map[uint32]*stream
	}

	stream struct {
		d *DB

		mu sync.Mutex

		io.Writer
		written int64
		events  int
		first   time.Time

		Labels tlog.Labels
		lsmap  map[string]struct{}
	}

	streamInfo struct {
		ID     uint32
		Labels tlog.Labels
	}
)

func NewDB(dir string) (*DB, error) {
	d := &DB{
		dir: dir,

		MaxFileSize:   32 * compress.MiB,
		MaxFileEvents: 1_000_000,

		s: make(map[uint32]*stream),
	}

	err := d.init()
	if err != nil {
		return nil, errors.Wrap(err, "init")
	}

	return d, nil
}

func (d *DB) init() (err error) {
	err = d.walk(d.dir)

	return errors.Wrap(err, "walk db files")
}

func (d *DB) walk(dir string) (err error) {
	err = filepath.WalkDir(d.dir, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return errors.Wrap(err, path)
		}

		if e.IsDir() {
			return nil
		}

		if e.Type().Type() == fs.ModeSymlink {
			err = d.walk(path)

			return errors.Wrap(err, "symlink: %v", path)
		}

		if !strings.HasSuffix(path, ".tlz") {
			return nil
		}

		tlog.Printw("read file", "path", path, "name", e.Name())

		return nil
	})

	return err
}

func (d *DB) Stream(ctx context.Context, w io.Writer, q string, start int64) (err error) {
	panic("stop")
}

func (d *DB) allLabels() tlog.Labels {
	defer d.mu.Unlock()
	d.mu.Lock()

	sum := map[string]struct{}{}

	for _, s := range d.s {
		s.mu.Lock()
		for ll := range s.lsmap {
			sum[ll] = struct{}{}
		}
		s.mu.Unlock()
	}

	ls := make(tlog.Labels, 0, len(sum))

	for ll := range sum {
		ls = append(ls, ll)
	}

	return ls
}

func (d *DB) allStreams() []streamInfo {
	defer d.mu.Unlock()
	d.mu.Lock()

	ss := make([]streamInfo, 0, len(d.s))

	for id, s := range d.s {
		s.mu.Lock()
		ss = append(ss, streamInfo{
			ID:     id,
			Labels: s.Labels,
		})
		s.mu.Unlock()
	}

	return ss
}

func (d *DB) Write(p []byte) (i int, err error) {
	var ev parse.Event

	ctx := context.Background()

	_, i, err = ev.Parse(ctx, p, 0)
	if err != nil {
		return 0, errors.Wrap(err, "parse event")
	}

	if ev.Timestamp == 0 {
		return 0, errors.Wrap(err, "no timestamp")
	}

	s, err := d.stream(&ev)
	if err != nil {
		return 0, errors.Wrap(err, "open file")
	}

	err = s.WriteEvent(p, &ev)
	if err != nil {
		return 0, errors.Wrap(err, "write")
	}

	return len(p), nil
}

func (d *DB) stream(ev *parse.Event) (s *stream, err error) {
	sum := crc32.ChecksumIEEE(ev.Labels)

	defer d.mu.Unlock()
	d.mu.Lock()

	s, ok := d.s[sum]
	if ok {
		return s, nil
	}

	s = &stream{
		d: d,

		lsmap: make(map[string]struct{}),
	}

	if len(ev.Labels) != 0 {
		var d wire.Decoder
		_ = s.Labels.TlogParse(&d, ev.Labels, 0)
	}

	for _, ll := range s.Labels {
		s.lsmap[ll] = struct{}{}
	}

	d.s[sum] = s

	return s, nil
}

func (s *stream) WriteEvent(p []byte, ev *parse.Event) (err error) {
	today := time.Unix(0, ev.Timestamp).Round(24 * time.Hour)

	defer s.mu.Unlock()
	s.mu.Lock()

	if s.Writer == nil {
		s.Writer, err = s.newFile(ev)
		tlog.Printw("open file", "ts", tlog.Timestamp(ev.Timestamp), "Labels", tlog.RawMessage(ev.Labels), "err", err)
		if err != nil {
			return errors.Wrap(err, "new file")
		}

		s.written = 0
		s.events = 0
		s.first = today
	}

	defer func() {
		if err == nil || s.Writer == nil {
			return
		}

		_ = s.closeWriter()
	}()

	n, err := s.Writer.Write(p)
	s.events++
	s.written += int64(n)
	if err != nil {
		return errors.Wrap(err, "write file")
	}

	if s.d.MaxFileSize != 0 && s.written >= s.d.MaxFileSize {
		err = s.closeWriter()
		tlog.Printw("close file by size", "written", s.written, "max_size", s.d.MaxFileSize, "err", err)
		if err != nil {
			return errors.Wrap(err, "close file")
		}
	}

	if s.d.MaxFileEvents != 0 && s.events >= s.d.MaxFileEvents {
		err = s.closeWriter()
		tlog.Printw("close file by events", "events", s.events, "max_events", s.d.MaxFileEvents, "err", err)
		if err != nil {
			return errors.Wrap(err, "close file")
		}
	}

	if s.first != today {
		err = s.closeWriter()
		tlog.Printw("close file by date", "today", today, "first", s.first, "err", err)
		if err != nil {
			return errors.Wrap(err, "close file")
		}
	}

	return nil
}

func (s *stream) closeWriter() (err error) {
	cl, ok := s.Writer.(io.Closer)
	if ok {
		err = cl.Close()
	}

	s.Writer = nil

	return err
}

func (s *stream) newFile(ev *parse.Event) (w io.Writer, err error) {
	sum := crc32.ChecksumIEEE(ev.Labels)

	t := time.Unix(0, ev.Timestamp)
	fn := t.Format("2006-01-02_15-04")

	full := filepath.Join(s.d.dir, fmt.Sprintf("%s_%08x.tlz", fn, sum))
	dir := filepath.Dir(full)

	err = os.MkdirAll(dir, 0755)
	if err != nil {
		return nil, errors.Wrap(err, "create dir")
	}

	f, err := os.OpenFile(full, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0744)
	if err != nil {
		return nil, errors.Wrap(err, "open file")
	}

	e := compress.NewEncoder(f, compress.MiB)

	wc := tlio.WriteCloser{
		Writer: e,
		Closer: f,
	}

	return wc, nil
}
