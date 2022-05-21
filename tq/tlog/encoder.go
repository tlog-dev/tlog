package tlog

import (
	"context"

	"github.com/nikandfor/errors"
	"github.com/nikandfor/tlog/wire"
)

type (
	Encoder struct {
		wire.Encoder
	}
)

func (e *Encoder) Append(ctx context.Context, b []byte, x interface{}) ([]byte, error) {
	switch x := x.(type) {
	case wire.TlogAppender:
		b = x.TlogAppend(&e.Encoder, b)
	default:
		return nil, errors.New("unsupported type: %T", x)
	}

	return b, nil
}
