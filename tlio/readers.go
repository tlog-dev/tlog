package tlio

import (
	"io"
	"os"

	"github.com/nikandfor/errors"
	"github.com/nikandfor/tlog"
)

type (
	ReadCloser struct {
		io.Reader
		io.Closer
	}

	Seeker interface {
		Seek(off int64, whence int) (int64, error)
	}

	ReadSeeker interface {
		io.Reader
		Seeker
	}

	ReReader struct {
		ReadSeeker

		Hook func(old, cur int64)

		pos int64
	}

	DumpReader struct {
		io.Reader

		tlog.Span

		Pos int64
	}
)

func NewReReader(r ReadSeeker) (*ReReader, error) {
	cur, err := r.Seek(0, os.SEEK_CUR)
	if err != nil {
		return nil, errors.Wrap(err, "seek")
	}

	return &ReReader{
		ReadSeeker: r,

		pos: cur,
	}, nil
}

func (r *ReReader) Read(p []byte) (n int, err error) {
	n, err = r.ReadSeeker.Read(p)

	r.pos += int64(n)

	if n == 0 && errors.Is(err, io.EOF) {
		end, err := r.ReadSeeker.Seek(0, os.SEEK_END)
		if err != nil {
			return n, errors.Wrap(err, "seek")
		}

		switch {
		case end < r.pos:
			if r.Hook != nil {
				r.Hook(r.pos, end)
			}

			r.pos = end
		case end > r.pos:
			_, err = r.ReadSeeker.Seek(r.pos, os.SEEK_SET)
			if err != nil {
				return n, errors.Wrap(err, "seek back")
			}
		}
	}

	return n, err
}

func (r *DumpReader) Read(p []byte) (n int, err error) {
	n, err = r.Reader.Read(p)

	r.Span.Printw("read", "n", n, "err", err, "p_len", len(p), "pos", r.Pos, "data", p[:n])

	r.Pos += int64(n)

	return
}
