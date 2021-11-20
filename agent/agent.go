package agent

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/nikandfor/errors"
	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/convert"
	"github.com/nikandfor/tlog/wire"
)

type (
	Agent struct {
		MaxEvents int

		mu sync.Mutex

		b []*msg
		w int

		clients map[net.Addr]*connWriter

		cond *sync.Cond
	}

	msg struct {
		ev    []byte
		ls    []byte
		lsmsg []byte
	}

	Conn struct {
		io.Reader
		io.Writer
		io.Closer
	}

	connWriter struct {
		a *Agent

		d wire.Decoder

		addr  net.Addr
		ls    []byte
		lsmsg []byte
	}
)

func New() (a *Agent) {
	a = &Agent{
		MaxEvents: 100000,
		clients:   make(map[net.Addr]*connWriter),
	}

	a.cond = sync.NewCond(&a.mu)

	return a
}

func (a *Agent) Subscribe(ctx context.Context, c io.Writer, n int, follow, addlabels bool) (err error) {
	var b []byte

	f, _ := c.(interface {
		Flush()
	})

	a.mu.Lock()

	r := a.w - n

	if r < a.w-len(a.b) {
		r = a.w - len(a.b)
	}

	if a.w > 0 {
		var m *msg

		if r == a.w {
			m = a.b[(a.w-1)%len(a.b)]
		} else {
			m = a.b[r%len(a.b)]
		}

		if len(m.lsmsg) != 0 {
			b = append(b[:0], m.lsmsg...)

			a.mu.Unlock()

			_, err = c.Write(b)
			if err != nil {
				return errors.Wrap(err, "write msg")
			}

			if f != nil {
				f.Flush()
			}

			a.mu.Lock()
		}
	}

	for {
		w := a.w

		if r == a.w {
			if !follow {
				break
			}

			a.cond.Wait()

			continue
		}

		if r < a.w-len(a.b) {
			r = a.w - len(a.b)
		}

		m := a.b[r%len(a.b)]

		if len(m.ls) != 0 && addlabels {
			b = convert.Set(b[:0], m.ev, m.ls)
		} else {
			b = append(b[:0], m.ev...)
		}

		a.mu.Unlock()

		_, _ = c.Write(b)

		if r+1 == w && f != nil {
			f.Flush()
			//	tlog.Printw("flushed", "r", r, "w", w)
		}

		//	tlog.Printw("written", "r", r, "w", w)

		a.mu.Lock()

		r++
	}

	a.mu.Unlock()

	return nil
}

func (a *Agent) addEvent(ev, ls, lsmsg []byte) {
	defer a.cond.Broadcast()

	defer a.mu.Unlock()
	a.mu.Lock()

	var m *msg
	w := a.w % a.MaxEvents

	if w < len(a.b) {
		m = a.b[w]
	} else if w < a.MaxEvents {
		m = new(msg)

		a.b = append(a.b, m)
	}

	a.w++

	m.ev = append(m.ev[:0], ev...)
	m.ls = ls
	m.lsmsg = lsmsg

	//	tlog.Printw("added", "ls", ls, "event", tlog.RawMessage(ev))
}

func (a *Agent) ListenPacket(p net.PacketConn) error {
	var buf [0xffff]byte
	for {
		n, addr, err := p.ReadFrom(buf[:])
		//	tlog.Printw("read packet", "n", n, "addr", addr, "err", err)
		if err != nil {
			return errors.Wrap(err, "read from")
		}

		w := a.Writer(addr)

		_, err = w.Write(buf[:n])
		if err != nil {
			tlog.Printw("process packet", "err", err)
		}
	}
}

func (a *Agent) Listen(l net.Listener) (err error) {
	errc := make(chan error, 1)

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				errc <- errors.Wrap(err, "accept")
				return
			}

			go func() {
				err := a.ServeConn(conn)

				if err != nil {
					tlog.Printw("serve conn", "err", err)
				}
				//errors.Wrap(err, "serve conn %v", conn.RemoteAddr())
			}()
		}
	}()

	return <-errc
}

func (a *Agent) ServeConn(c net.Conn) (err error) {
	w := a.Writer(c.RemoteAddr())

	r := wire.NewStreamDecoder(c)

	_, err = r.WriteTo(w)
	if err != nil {
		return errors.Wrap(err, "copy")
	}

	return nil
}

func (a *Agent) newWriter(addr net.Addr) (c *connWriter) {
	return &connWriter{
		a:    a,
		addr: addr,
	}
}

func (a *Agent) Writer(addr net.Addr) (c *connWriter) {
	defer a.mu.Unlock()
	a.mu.Lock()

	c, ok := a.clients[addr]

	if !ok {
		c = a.newWriter(addr)

		a.clients[addr] = c
	}

	return c
}

func (w *connWriter) Write(p []byte) (i int, err error) {
	ls, err := w.findLabels(p)
	if err != nil {
		return 0, err
	}

	w.a.addEvent(p, w.ls, w.lsmsg)

	if ls != nil {
		w.ls = append([]byte{}, ls...)
		w.lsmsg = append([]byte{}, p...)
	}

	return
}

func (w *connWriter) findLabels(p []byte) (ls []byte, err error) {
	defer func() {
		p := recover()
		if p == nil {
			return
		}

		switch p := p.(type) {
		case error:
			err = p
		case string:
			err = errors.NewNoLoc(p)
		default:
			err = fmt.Errorf("%v", p)
		}

		err = errors.Wrap(err, "parse message")
	}()

	tag, els, i := w.d.Tag(p, 0)
	if tag != wire.Map {
		return
	}

	var e tlog.EventKind

	var k []byte
	var sub int64
	for el := 0; els == -1 || el < int(els); el++ {
		if els == -1 && w.d.Break(p, &i) {
			break
		}

		st := i

		k, i = w.d.String(p, i)

		tag = w.d.TagOnly(p, i)
		if tag != wire.Semantic {
			i = w.d.Skip(p, i)

			continue
		}

		tag, sub, _ = w.d.Tag(p, i)

		switch {
		case sub == tlog.WireEventKind && string(k) == tlog.KeyEventKind:
			i = e.TlogParse(&w.d, p, i)
		case sub == tlog.WireLabels && string(k) == tlog.KeyLabels:
			end := w.d.Skip(p, i)
			ls = p[st:end]
			i = end
		default:
			i = w.d.Skip(p, i)
		}
	}

	if e == tlog.EventLabels {
		w.ls = nil
		w.lsmsg = nil
	} else {
		ls = nil
	}

	return ls, nil
}
