package sub

import (
	"context"

	"github.com/nikandfor/tlog"
)

func Func(ctx context.Context, i int) {
	tr := tlog.SpawnFromContext(ctx)
	defer tr.Finish()

	tr.Printf("sub.func: %d (traced)", i)
}
