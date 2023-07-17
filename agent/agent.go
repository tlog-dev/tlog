package agent

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/nikandfor/errors"
	"tlog.app/go/eazy"

	"tlog.app/go/tlog"
	"tlog.app/go/tlog/tlio"
	"tlog.app/go/tlog/tlwire"
)

type (
	Agent struct { //nolint:maligned
		path string

		sync.Mutex

		files []*file
		subs  map[int64]*sub
		subid int64

		// end of Mutex

		Partition   time.Duration
		MaxFileSize int64

		KeyTimestamp string

		Stderr io.Writer

		d tlwire.Decoder
	}

	file struct {
		sum    uint32
		labels []byte
		start  time.Time

		sync.Mutex

		io.Writer

		// end of Mutex

		a *Agent
	}

	sub struct {
		id int64
		io.Writer
	}
)

func New(db string) (*Agent, error) {
	a := &Agent{
		path: db,

		subs: make(map[int64]*sub),

		Partition:    3 * time.Hour,
		KeyTimestamp: tlog.KeyTimestamp,

		Stderr: os.Stderr,
	}

	err := a.openFiles()
	if err != nil {
		return a, errors.Wrap(err, "open files")
	}

	return a, nil
}

func (a *Agent) openFiles() (err error) {
	err = filepath.WalkDir(a.path, func(path string, d fs.DirEntry, err error) error {
		if path == a.path {
			return nil
		}
		if d.IsDir() {
			return fs.SkipDir
		}

		base := filepath.Base(path)
		ext := filepath.Ext(path)

		if ext != ".tlz" || !strings.HasPrefix(base, "events_") {
			return nil
		}

		ff, err := os.Open(path)
		if err != nil {
			return errors.Wrap(err, "open file")
		}

		defer func() {
			e := ff.Close()
			if err == nil {
				err = errors.Wrap(e, "close file")
			}
		}()

		dec := eazy.NewReader(ff)
		sd := tlwire.NewStreamDecoder(dec)

		msg, err := sd.Decode()
		if err != nil {
			return errors.Wrap(err, "decode message")
		}

		_, err = a.getFile(msg)
		if err != nil {
			return errors.Wrap(err, "get file")
		}

		return nil
	})

	return nil
}

func (a *Agent) Write(p []byte) (n int, err error) {
	defer func() {
		perr := recover()

		if err == nil && perr == nil {
			return
		}

		if perr != nil {
			fmt.Fprintf(a.Stderr, "panic: %v (pos %x)\n", perr, n)
		} else {
			fmt.Fprintf(a.Stderr, "parse error: %+v (pos %x)\n", err, n)
		}
		fmt.Fprintf(a.Stderr, "dump\n%v", tlwire.Dump(p))
		fmt.Fprintf(a.Stderr, "hex dump\n%v", hex.Dump(p))

		s := debug.Stack()
		fmt.Fprintf(a.Stderr, "%s", s)
	}()

	f, err := a.getFile(p)
	if err != nil {
		return 0, err
	}

	n, err = f.Write(p)

	func() {
		defer a.Unlock()
		a.Lock()

		for _, sub := range a.subs {
			_, _ = sub.Writer.Write(p)
		}
	}()

	return
}

func (a *Agent) Close() (err error) {
	a.Lock()
	defer a.Unlock()

	for _, f := range a.files {
		e := f.Close()
		if err == nil {
			err = errors.Wrap(e, "file %v", f.sum)
		}
	}

	return err
}

func (a *Agent) getFile(p []byte) (f *file, err error) {
	if tlog.If("dump") {
		defer func() {
			var sum uint32
			if f != nil {
				sum = f.sum
			}

			tlog.Printw("message", "sum", tlog.NextAsHex, sum, "msg", tlog.RawMessage(p))
		}()
	}

	defer a.Unlock()
	a.Lock()

	ts, labels, err := a.parseEventHeader(p)
	if err != nil {
		return nil, errors.Wrap(err, "parse event header")
	}

	start := time.Unix(0, ts).UTC().Truncate(a.Partition)

	f = a.getPart(labels, start)

	return f, nil
}

func (a *Agent) getPart(labels []byte, start time.Time) *file {
	sum := crc32.ChecksumIEEE(labels)

	for _, f := range a.files {
		if f.sum == sum && f.start.UnixNano() == start.UnixNano() && bytes.Equal(f.labels, labels) {
			return f
		}
	}

	f := &file{
		sum:    sum,
		labels: append([]byte{}, labels...),
		start:  start,

		a: a,
	}

	a.files = append(a.files, f)

	tlog.Printw("open file", "sum", tlog.NextAsHex, sum, "labels", tlog.RawTag(tlwire.Map, -1), tlog.RawMessage(labels), tlog.Break)

	return f
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

func (f *file) Write(p []byte) (n int, err error) {
	defer f.Unlock()
	f.Lock()

	if f.Writer == nil {
		f.Writer, err = f.a.openWriter(f)
		if err != nil {
			return 0, errors.Wrap(err, "open writer")
		}
	}

	n, err = f.Writer.Write(p)

	return
}

func (f *file) Close() (err error) {
	if c, ok := f.Writer.(io.Closer); ok {
		e := c.Close()
		if err == nil {
			err = errors.Wrap(e, "close writer")
		}
	}

	return err
}

func (a *Agent) openWriter(f *file) (io.Writer, error) {
	name := filepath.Join(a.path, fmt.Sprintf("events_%08x_%s.tlz", f.sum, f.start.Format("2006-01-02T15-04")))
	dir := filepath.Dir(name)

	err := os.MkdirAll(dir, 0o755)
	if err != nil {
		return nil, errors.Wrap(err, "mkdir")
	}

	ff, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return nil, errors.Wrap(err, "open file")
	}

	w := eazy.NewWriter(ff, eazy.MiB, 2*1024)

	return tlio.WriteCloser{
		Writer: w,
		Closer: ff,
	}, nil
}
