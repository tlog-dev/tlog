package tlclick

import (
	"context"
	"crypto/tls"

	ch "github.com/ClickHouse/ch-go"
	"github.com/ClickHouse/ch-go/chpool"
)

func NewPool(ctx context.Context, opts chpool.Options) (*chpool.Pool, error) { //nolint:gocritic
	return chpool.New(ctx, opts)
}

func DefaultPoolOptions(addr string) chpool.Options {
	return chpool.Options{
		ClientOptions: ch.Options{
			Address:     addr,
			Compression: ch.CompressionZSTD,
			ClientName:  "tlog agent",

			TLS: &tls.Config{
				MinVersion: tls.VersionTLS13,
			},
		},
	}
}
