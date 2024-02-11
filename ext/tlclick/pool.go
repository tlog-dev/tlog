package tlclick

import (
	"context"
	"crypto/tls"

	click "github.com/ClickHouse/ch-go"
	"github.com/ClickHouse/ch-go/chpool"
)

func NewPool(ctx context.Context, opts chpool.Options) (*chpool.Pool, error) {
	return chpool.New(ctx, opts)
}

func DefaultPoolOptions(addr string) chpool.Options {
	return chpool.Options{
		ClientOptions: click.Options{
			Address:     addr,
			Compression: click.CompressionZSTD,
			ClientName:  "tlog agent",

			TLS: &tls.Config{},
		},
	}
}
