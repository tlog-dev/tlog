package agent

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/nikandfor/errors"
	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/compress"
	"github.com/nikandfor/tlog/low"
	"github.com/nikandfor/tlog/tlio"
	"github.com/nikandfor/tlog/tq/parse"
	"github.com/nikandfor/tlog/wire"
)

type (
	Stream struct {
		fs  fs.FS
		fsw OpenFiler

		log  *tlog.Logger
		logf io.Closer

		mu sync.RWMutex

		files []*file

		w    io.Writer
		file *file

		c    *compress.Encoder
		cbuf low.Buf

		Partition   time.Duration
		MaxFileSize int64
		MaxDataSize int64
		MaxEvents   int

		MaxBlockEvents int

		CompressorBlockSize int

		TimeFormat string

		lastPart time.Time
		lastTry  int

		lastTs int64
	}

	file struct {
		name string

		first int64
		last  int64

		blocks []block

		part   time.Time
		dsize  int64
		fsize  int64
		events int
	}

	block struct {
		foff int64
		doff int64
		ts   int64
	}

	StreamReader struct {
		s *Stream

		part time.Time
		doff int64

		rc io.ReadCloser
		sd *wire.StreamDecoder
	}

	skipper struct {
		ts  int64
		err error
	}
)

func NewStream(fs fs.FS) (s *Stream, err error) {
	s = &Stream{
		fs: fs,

		Partition:   24 * time.Hour,
		MaxFileSize: 16 * compress.GiB,
		MaxDataSize: 128 * compress.GiB,
		MaxEvents:   16_000_000,

		MaxBlockEvents: 16_000,

		CompressorBlockSize: compress.MiB,

		TimeFormat: "2006-01-02_15-04",
	}

	s.fsw, _ = fs.(OpenFiler)

	err = s.openLog()
	if err != nil {
		return nil, errors.Wrap(err, "open log")
	}

	return s, nil
}

func (s *Stream) Close() (err error) {
	defer s.mu.Unlock()
	s.mu.Lock()

	if s.w != nil {
		e := s.closeWriter()
		if err == nil {
			err = errors.Wrap(e, "close file")
		}
	}

	e := s.logf.Close()
	if err == nil {
		err = errors.Wrap(e, "close log")
	}

	return err
}

func (s *Stream) openLog() (err error) {
	var f io.Reader
	var w io.ReadWriteCloser

	if s.fsw != nil {
		w, err = s.fsw.OpenFile("log.tlz", os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
		f = w
	} else {
		f, err = s.fs.Open("log.tlz")
	}

	if err != nil {
		return errors.Wrap(err, "open log")
	}

	d := compress.NewDecoder(f)

	// read log
	sd := wire.NewStreamDecoder(d)

	lr := tlio.NewTeeWriter(tlog.DefaultLogger, tlio.WriterFunc(s.logReplay))

	_, err = sd.WriteTo(lr)
	if err != nil {
		return errors.Wrap(err, "read log")
	}

	if s.fsw == nil {
		return nil
	}

	// create logger

	e := compress.NewEncoder(w, 16*compress.KiB)
	s.log = tlog.New(e)

	s.log.NoCaller = true

	s.logf = w

	return nil
}

func (s *Stream) logReplay(p []byte) (n int, err error) {
	var ev parse.LowEvent

	n, err = ev.Parse(p, n)
	if err != nil {
		return 0, errors.Wrap(err, "parse event")
	}

	var msg []byte
	var fname []byte
	var part time.Time
	var first, last int64
	var fsize, dsize int64
	var events int

	var d wire.Decoder

	for i := 0; i < ev.Len(); i++ {
		key, val := ev.Index(i)

		switch string(key) {
		case tlog.KeyTime:
			// skip
		case tlog.KeyMessage:
			_, _, j := d.Tag(val, 0)
			msg, j = d.String(val, j)
			if j != len(val) {
				panic("msg")
			}
		case "fname":
			fname, _ = d.String(val, 0)
		case "part":
			part, _ = d.Time(val, 0)
		case "first_ts":
			first, _ = d.Timestamp(val, 0)
		case "last_ts":
			last, _ = d.Timestamp(val, 0)
		case "data_size":
			dsize, _ = d.Signed(val, 0)
		case "file_size":
			fsize, _ = d.Signed(val, 0)
		case "events":
			var x int64
			x, _ = d.Signed(val, 0)
			events = int(x)
		default:
			tlog.Printw("unrecognized key", "key", key, "val", tlog.RawMessage(val))
		}
	}

	switch string(msg) {
	case "new_file":
		s.file = &file{
			name:  string(fname),
			part:  part,
			first: first,
		}

		s.files = append(s.files, s.file)
	case "close_file":
		s.file.last = last
		s.file.fsize = fsize
		s.file.dsize = dsize
		s.file.events = events
	default:
		tlog.Printw("unsupported message", "msg", msg)
	}

	return n, nil
}

func (s *Stream) Write(p []byte) (n int, err error) {
	var ev parse.LowEvent

	n, err = ev.Parse(p, n)
	if err != nil {
		return 0, errors.Wrap(err, "parse event")
	}

	err = s.WriteEvent(&ev)
	if err != nil {
		return 0, err
	}

	return n, nil
}

func (s *Stream) WriteEvent(ev *parse.LowEvent) (err error) {
	if s.fsw == nil {
		return errors.New("read-only stream (%T)", s.fs)
	}

	defer s.mu.Unlock()
	s.mu.Lock()

	// TODO: what if time is not ascending?

	if ev.Timestamp() <= s.lastTs {
		return errors.New("not ascending timestamp")
	}

	part := time.Unix(0, ev.Timestamp()).Truncate(s.Partition)

	if s.w != nil && s.file != nil && s.file.part != part {
		err = s.closeWriter()
		if err != nil {
			return errors.Wrap(err, "close file")
		}
	}

	if s.w != nil && s.file != nil && s.file.dsize+int64(len(ev.Bytes())) >= s.MaxDataSize {
		err = s.closeWriter()
		if err != nil {
			return errors.Wrap(err, "close file")
		}
	}

	// compress

	msg := ev.Bytes()

	if s.CompressorBlockSize != 0 {
		if s.c == nil {
			s.c = compress.NewEncoder(&s.cbuf, s.CompressorBlockSize)
		}

		s.cbuf = s.cbuf[:0]
		_, err = s.c.Write(ev.Bytes())
		if err != nil {
			return errors.Wrap(err, "compress")
		}

		msg = s.cbuf
	}

	if s.w != nil && s.file != nil && s.file.fsize+int64(len(s.cbuf)) >= s.MaxFileSize {
		err = s.closeWriter()
		if err != nil {
			return errors.Wrap(err, "close file")
		}
	}

	if s.w == nil {
		err = s.newFile(part, ev.Timestamp())
		if err != nil {
			return errors.Wrap(err, "new file")
		}

		if s.CompressorBlockSize != 0 {
			s.c.Reset(&s.cbuf)

			s.cbuf = s.cbuf[:0]
			_, err = s.c.Write(ev.Bytes())
			if err != nil {
				return errors.Wrap(err, "compress")
			}

			msg = s.cbuf
		}
	}

	// write compressed
	n, err := s.w.Write(msg)
	s.file.fsize += int64(n)
	if err != nil {
		return errors.Wrap(err, "write")
	}

	s.file.dsize += int64(len(ev.Bytes()))
	s.file.events++

	s.lastTs = ev.Timestamp()

	if s.file.events >= s.MaxEvents {
		err = s.closeWriter()
		if err != nil {
			return errors.Wrap(err, "close file")
		}
	}

	return nil
}

func (s *Stream) closeWriter() (err error) {
	cl, ok := s.w.(io.Closer)
	if ok {
		err = cl.Close()
	}

	err = s.log.Event(
		tlog.KeyTime, time.Now(),
		tlog.KeyMessage, tlog.Message("close_file"),
		"last_ts", tlog.Timestamp(s.lastTs),
		"data_size", s.file.dsize,
		"file_size", s.file.fsize,
		"events", s.file.events,
	)
	if err != nil {
		return errors.Wrap(err, "write log")
	}

	s.w = nil
	s.file = nil

	return err
}

func (s *Stream) newFile(part time.Time, ts int64) (err error) {
	if s.w != nil {
		panic("not closed file?")
	}

	if part == s.lastPart {
		s.lastTry++
	} else {
		s.lastPart = part
		s.lastTry = 0
	}

	tm := time.Unix(0, ts).Format(s.TimeFormat)

	var base string
	if s.lastTry == 0 {
		base = fmt.Sprintf("%s.tlz", tm)
	} else {
		base = fmt.Sprintf("%s_%04x.tlz", tm, s.lastTry)
	}

	f, err := s.fsw.OpenFile(base, os.O_CREATE|os.O_EXCL|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return errors.Wrap(err, "open file")
	}

	ff := &file{
		name:  base,
		part:  part,
		first: ts,
	}

	s.files = append(s.files, ff)
	s.file = ff

	err = s.log.Event(
		tlog.KeyTime, time.Now(),
		tlog.KeyMessage, tlog.Message("new_file"),
		"fname", base,
		"part", part,
		"first_ts", tlog.Timestamp(ts),
	)
	if err != nil {
		return errors.Wrap(err, "write log")
	}

	s.w = f

	return nil
}

func (s *Stream) findFile(t time.Time) *file {
	if len(s.files) == 0 {
		return nil
	}

	i := sort.Search(len(s.files), func(i int) bool {
		return !s.files[i].part.Before(t)
	})
	if i < len(s.files) {
		return s.files[i]
	}

	return nil
}

func (s *Stream) OpenReader() (*StreamReader, error) {
	r := &StreamReader{
		s: s,
	}

	return r, nil
}

func (r *StreamReader) WriteTo(w io.Writer) (n int64, err error) {
	if r.sd == nil {
		file := r.s.findFile(r.part)
		if file == nil {
			return 0, nil
		}

		f, err := r.s.fs.Open(file.name)
		if err != nil {
			return 0, errors.Wrap(err, "open file")
		}

		var rd io.Reader = f

		if r.s.CompressorBlockSize != 0 {
			rd = compress.NewDecoder(rd)
		}

		r.rc = f
		r.sd = wire.NewStreamDecoder(rd)
	}

	n, err = r.sd.WriteTo(w)
	r.doff += n

	return
}

func (r *StreamReader) Close() error {
	if r.rc != nil {
		return r.rc.Close()
	}

	return nil
}

func (s skipper) Write(p []byte) (_ int, err error) {
	var ev parse.LowEvent

	_, err = ev.Parse(p, 0)
	if err != nil {
		return 0, err
	}

	if ev.Timestamp() < s.ts {
		return len(p), nil
	}

	return 0, s.err
}
