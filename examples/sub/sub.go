package sub

import (
	"context"

	"github.com/nikandfor/tlog"
)

func Func1(id tlog.FullID, i int) {
	tr := tlog.Spawn(id)
	defer tr.Finish()

	tr.Printf("sub.func1: %d (traced)", i)

	tlog.Printf("sub.func1: %d", i)
}

func Func2(ctx context.Context, i int) {
	tr := tlog.SpawnFromContext(ctx)
	defer tr.Finish()

	tr.Printf("sub.func2: %d (traced)", i)
}
