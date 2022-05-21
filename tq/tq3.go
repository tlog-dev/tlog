package tq

import "context"

type (
	any = interface{}

	Dot struct{}

	Array struct {
		Of any
	}
)

func (f Array) Eval(ctx context.Context, arg any) (z any, err error) {
	//	qq.Eval()
	return
}
