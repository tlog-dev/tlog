package sub

import (
	"context"

	"tlog.app/go/tlog"
)

func Func(ctx context.Context, i int) {
	tr := tlog.SpawnFromContext(ctx, "sub_routine")
	defer tr.Finish()

	tr.Printf("sub.func: %d (traced)", i)
}
