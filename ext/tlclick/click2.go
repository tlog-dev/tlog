package tlclick

import (
	"context"
	"io"

	"github.com/ClickHouse/ch-go/chpool"
)

type (
	Click struct {
		pool *chpool.Pool
	}
)

func New(pool *chpool.Pool) *Click {
	return &Click{pool: pool}
}

func (d *Click) Write(p []byte) (int, error) {
	return len(p), nil
}

func (d *Click) Query(ctx context.Context, w io.Writer, ts int64, q string) error { return nil }

func (d *Click) CreateTables(ctx context.Context) error { return nil }
