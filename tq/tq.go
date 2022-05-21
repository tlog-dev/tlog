//go:build ignore

package tq

import (
	"errors"
	"io"

	"github.com/nikandfor/tlog/wire"
)

type (
	Keys struct {
		wire.State

		e wire.Encoder

		r io.ReadCloser

		val bool
	}

	Key struct {
		wire.State

		e wire.Encoder

		Key string

		r io.ReadCloser

		val bool
	}
)

func (f *Keys) Read(p []byte) (n int, err error) {
	if f.r == nil {
		var ss *wire.SubState

		ss, n, err = f.SubState(p)
		if err != nil {
			return 0, err
		}

		if ss.Tag != wire.Map {
			return 0, errors.New("map expected")
		}

		_ = f.e.AppendArray(p[:0], int(ss.Sub))

		f.r = ss
	}

	//	defer func() {
	//		println(fmt.Sprintf("keys  %x => %x %v  %x %q", len(p), n, err, p[:n], p[:n]))
	//	}()

	var m int
	for n < len(p) {
		m, err = f.r.Read(p[n:])
		//	println(fmt.Sprintf("read  var %5v  %x + %x  err %v  % x  %q", f.val, n, m, err, p[n:n+m], p[n:n+m]))
		if f.val {
			if err != nil {
				return
			}

			f.val = false

			continue
		}

		n += m

		if err == io.EOF {
			_ = f.r.Close()
			f.r = nil

			return n, nil
		}

		if err != nil {
			return
		}

		f.val = true
	}

	return n, io.ErrShortBuffer
}

func (f *Key) Read(p []byte) (n int, err error) {
	if f.r == nil {
		var ss *wire.SubState

		ss, _, err = f.SubState(p)
		if err != nil {
			return 0, err
		}

		if ss.Tag != wire.Map {
			return 0, errors.New("map expected")
		}

		f.r = ss
	}

	m, err := f.Read(p[n:])
	if f.s == 0 {
	}

	if err == io.EOF {
		err = nil
	}
	if err != nil {
		return
	}

	//

	var m int
	for n < len(p) {
		m, err = f.r.Read(p[n:])
		//	println(fmt.Sprintf("read  var %5v  %x + %x  err %v  % x  %q", f.val, n, m, err, p[n:n+m], p[n:n+m]))
		if f.val {
			if err != nil {
				return
			}

			f.val = false

			continue
		}

		n += m

		if err == io.EOF {
			_ = f.r.Close()
			f.r = nil

			return n, nil
		}

		if err != nil {
			return
		}

		f.val = true
	}

	return n, io.ErrShortBuffer
}
