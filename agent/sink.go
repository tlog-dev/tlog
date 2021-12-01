package agent

import (
	"context"
	"io"
	"net"

	"github.com/nikandfor/errors"
	"github.com/nikandfor/tlog/wire"
)

type (
	Sink struct {
		io.Writer
	}
)

func NewSink(w io.Writer) *Sink {
	return &Sink{Writer: w}
}

func (w *Sink) Listen(ctx context.Context, l net.Listener) (err error) {
	for {
		conn, err := l.Accept()
		if err != nil {
			return errors.Wrap(err, "accept")
		}

		go func() {
			defer func() {
				_ = conn.Close()
			}()

			_ = w.HandleConn(ctx, conn)
		}()
	}
}

func (w *Sink) HandleConn(ctx context.Context, c net.Conn) (err error) {
	if r, ok := w.Writer.(io.ReaderFrom); ok {
		_, err = r.ReadFrom(c)
		return errors.Wrap(err, "read from")
	}

	d := wire.NewStreamDecoder(c)

	_, err = d.WriteTo(w)
	if err != nil {
		return errors.Wrap(err, "write to")
	}

	return nil
}

func (w *Sink) ListenPacket(ctx context.Context, p net.PacketConn) (err error) {
	b := make([]byte, 0x10000)

	for {
		n, _, err := p.ReadFrom(b)
		if err != nil {
			return errors.Wrap(err, "read from")
		}

		_, _ = w.Write(b[:n])
	}
}
