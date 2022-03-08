package agent

import (
	"context"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nikandfor/errors"
	"github.com/nikandfor/loc"
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
		Partition     time.Duration

		mu sync.Mutex

		s map[uint32]*stream
	}

	stream struct {
		d *DB

		mu sync.Mutex

		io.Writer
		written int64
		events  int
		part    time.Time

		Labels tlog.Labels
		lsmap  map[string]struct{}
		ls     []byte

		notify chan struct{}
	}

	streamInfo struct {
		ID     string
		Labels tlog.Labels
	}
)

func NewDB(dir string) (*DB, error) {
	d := &DB{
		dir: dir,

		MaxFileSize:   32 * compress.MiB,
		MaxFileEvents: 1_000_000,
		Partition:     time.Hour, //24 * time.Hour,

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
	files, err := os.ReadDir(dir)
	if err != nil {
		return errors.Wrap(err, "read dir")
	}

	for _, f := range files {
		if !f.IsDir() {
			continue
		}

		x, err := strconv.ParseUint(f.Name(), 16, 32)
		if err != nil {
			return errors.Wrap(err, "parse stream id")
		}

		sid := uint32(x)

		// TODO: get labels somewhere
		ss := d.makeStream(nil)

		d.s[sid] = ss

		tlog.Printw("open stream", "sid", tlog.Hex(sid), "Labels", "__nope__")
	}

	return err
}

func (d *DB) Stream(ctx context.Context, w io.Writer, start int64, sid uint32) (err error) {
	tr := tlog.SpanFromContext(ctx)
	defer func() {
		tr.Printw("finished stream", "from", loc.Caller(1))
	}()

	s := d.getstream(sid)
	if s == nil {
		return errors.New("no such stream")
	}

	dir := filepath.Join(d.dir, fmt.Sprintf("%08x", sid))

	files, err := os.ReadDir(dir)
	if err != nil {
		return errors.Wrap(err, "read stream files")
	}

	i, err := d.findFile(files, start)
	if err != nil {
		return errors.Wrap(err, "find file")
	}

	tr.V("read_files").Printw("start position", "i", i, "files", len(files))

	if i < 0 {
		i = 0
	}

	var rc io.ReadCloser
	var sd *wire.StreamDecoder
	var n int64

	defer func() {
		if rc == nil {
			return
		}

		e := rc.Close()
		tr.V("read_files").Printw("close current file", "err", e)
		if err == nil {
			err = errors.Wrap(e, "close reader")
		}
	}()

	for err == nil && i <= len(files) {
		tr.V("read_files").Printw("current file", "i", i, "files", len(files))
		if i == len(files) {
			files1, err := os.ReadDir(dir)
			if err != nil {
				return errors.Wrap(err, "read stream files")
			}

			for j := len(files1) - 1; j >= 0; j-- {
				if files1[j] == files[i-1] {
					files = files1[j:]
					i = 1
					break
				}
			}

			tr.V("read_files").Printw("reread files", "i", i, "files", len(files))
		}

		if i < len(files) {
			if rc != nil {
				err = rc.Close()
				tr.V("open_files,read_files").Printw("close current file", "err", err)
				if err != nil {
					return errors.Wrap(err, "close reader")
				}
			}

			rc, err = d.openFile(filepath.Join(dir, files[i].Name()))
			if err != nil {
				return errors.Wrap(err, "open reader")
			}

			tr.V("open_files,read_files").Printw("open file", "file", files[i].Name())

			i++

			sd = wire.NewStreamDecoder(rc)
			n = 0
		}

		var m int64
		m, err = sd.WriteTo(w)
		n += m

		tr.V("read_files").Printw("copied", "m", tlog.Hex(m), "n", tlog.Hex(n), "err", err)

		if errors.Is(err, io.EOF) {
			err = nil
		}
		if err != nil {
			return errors.Wrap(err, "copy")
		}

		if i == len(files) {
			if f, ok := w.(tlio.Flusher); ok {
				tr.V("flusher").Printw("writer is flusher", "ok", ok)

				err = f.Flush()
				if err != nil {
					return errors.Wrap(err, "flush")
				}
			}

			tr.V("read_files").Printw("wait for updates")
			select {
			case <-s.notify:
			case <-ctx.Done():
				return nil
			}
		}
	}

	if err != nil {
		return
	}

	return nil
}

func (d *DB) findFile(files []fs.DirEntry, start int64) (i int, err error) {
	lo := "2006-01-02_15-04"

	for i = len(files) - 1; i >= 0; i-- {
		f := files[i]

		if f.IsDir() {
			continue
		}

		ext := filepath.Ext(f.Name())
		if ext != ".tlz" {
			continue
		}

		base := strings.TrimSuffix(f.Name(), ext)

		ts, err := time.Parse(lo, base[:len(lo)])
		if err != nil {
			return -1, errors.Wrap(err, "parse file name")
		}

		if ts.UnixNano() > start {
			continue
		}

		break
	}

	return
}

func (d *DB) openFile(file string) (rc io.ReadCloser, err error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, errors.Wrap(err, "open file")
	}

	var r io.Reader = f

	ext := filepath.Ext(file)
	switch ext {
	case ".tlz":
		r = compress.NewDecoder(r)
	}

	if r == f {
		return f, nil
	}

	rc = tlio.ReadCloser{
		Reader: r,
		Closer: f,
	}

	return rc, nil
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
			ID:     fmt.Sprintf("%08x", id),
			Labels: s.Labels,
		})
		s.mu.Unlock()
	}

	return ss
}

func (d *DB) Write(p []byte) (n int, err error) {
	var ev parse.LowEvent

	parser := parse.LowParser{
		New: func() *parse.LowEvent { return &ev },
	}

	ctx := context.Background()

	_, n, err = parser.Parse(ctx, p, n)
	if err != nil {
		return 0, errors.Wrap(err, "parse event")
	}
	if n != len(p) {
		return n, errors.New("parse: partial read")
	}

	if ev.Timestamp() == 0 {
		return 0, errors.Wrap(err, "no timestamp")
	}

	first, merged := parseLabels(ev.Labels())

	if len(first) == 0 {
		tlog.Printw("no labels", "ev_labels_len", len(ev.Labels()))
	}

	s, err := d.stream(&ev, first)
	if err != nil {
		return 0, errors.Wrap(err, "get stream")
	}

	err = s.WriteEvent(&ev)
	if err != nil {
		return 0, errors.Wrap(err, "write")
	}

	_ = merged // ignore additional labels

	return n, nil
}

func (d *DB) getstream(sid uint32) *stream {
	defer d.mu.Unlock()
	d.mu.Lock()

	return d.s[sid]
}

func (d *DB) stream(ev *parse.LowEvent, ls []byte) (s *stream, err error) {
	sum := crc32.ChecksumIEEE(ls)

	defer d.mu.Unlock()
	d.mu.Lock()

	s, ok := d.s[sum]
	if ok {
		if s.ls == nil { // TODO
			s.ls = append(s.ls, ls...)
		}
		return s, nil
	}

	s = d.makeStream(ls)

	if len(ls) != 0 {
		var d wire.Decoder
		_ = s.Labels.TlogParse(&d, ls, 0)
	}

	for _, ll := range s.Labels {
		s.lsmap[ll] = struct{}{}
	}

	d.s[sum] = s

	return s, nil
}

func (d *DB) makeStream(ls []byte) (s *stream) {
	s = &stream{
		d:     d,
		lsmap: make(map[string]struct{}),

		notify: make(chan struct{}),
	}

	if ls != nil {
		s.ls = append(s.ls[:0], ls...)
	}

	return
}

func (s *stream) WriteEvent(ev *parse.LowEvent) (err error) {
	defer s.mu.Unlock()
	s.mu.Lock()

	part := time.Unix(0, ev.Timestamp()).Round(s.d.Partition)

	if s.part != part && s.Writer != nil {
		err = s.closeWriter()
		tlog.Printw("close file by date", "cur_part", part, "file_part", s.part, "err", err)
		if err != nil {
			return errors.Wrap(err, "close file")
		}
	}

	if s.Writer == nil {
		s.Writer, err = s.newFile(ev, s.ls)
		if err != nil {
			return errors.Wrap(err, "new file")
		}

		s.written = 0
		s.events = 0
		s.part = part
	}

	defer func() {
		if err == nil || s.Writer == nil {
			return
		}

		_ = s.closeWriter()
	}()

	defer func() {
		for {
			select {
			case s.notify <- struct{}{}:
			default:
				return
			}
		}
	}()

	n, err := s.Writer.Write(ev.Bytes())
	s.events++
	s.written += int64(n)
	tlog.V("read_files").Printw("write file", "l", tlog.Hex(s.written))
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

func (s *stream) newFile(ev *parse.LowEvent, ls []byte) (w io.Writer, err error) {
	sum := crc32.ChecksumIEEE(ls)

	str := fmt.Sprintf("%08x", sum)

	t := time.Unix(0, ev.Timestamp())
	fn := t.Format("2006-01-02_15-04")

	try := 0

	base := fmt.Sprintf("%s.tlz", fn)

again:
	full := filepath.Join(s.d.dir, str, base)
	dir := filepath.Dir(full)

	err = os.MkdirAll(dir, 0755)
	if err != nil {
		return nil, errors.Wrap(err, "create dir")
	}

	f, err := os.OpenFile(full, os.O_CREATE|os.O_EXCL|os.O_WRONLY|os.O_APPEND, 0744)
	if os.IsExist(err) {
		try++
		base = fmt.Sprintf("%s_%04x.tlz", fn, try)
		goto again
	}
	if err != nil {
		return nil, errors.Wrap(err, "open file")
	}

	tlog.Printw("open file", "sid", tlog.Hex(sum), "Labels", tlog.RawMessage(ls), "name", filepath.Join(str, base), "err", err)

	e := compress.NewEncoder(f, compress.MiB)

	wc := tlio.WriteCloser{
		Writer: e,
		Closer: f,
	}

	return wc, nil
}

func parseLabels(p []byte) (first, merged []byte) {
	var d wire.Decoder

	i := d.Skip(p, 0)
	if i == len(p) {
		return p, nil
	}

	var e wire.Encoder

	first = p[:i]
	merged = make([]byte, 0, len(p))
	merged = e.AppendArray(merged, -1)

	i = 0
	var tag byte
	var sub int64

	for i < len(p) {
		tag, sub, i = d.Tag(p, i)
		if tag != wire.Semantic || sub != tlog.WireLabels {
			panic("not labels")
		}

		tag, sub, i = d.Tag(p, i)
		if tag != wire.Array {
			panic("bad labels")
		}

		st := i

		for el := 0; sub == -1 || el < int(sub); el++ {
			if sub == -1 && d.Break(p, &i) {
				break
			}

			i = d.Skip(p, i)
		}

		merged = append(merged, p[st:i]...)
	}

	return
}
