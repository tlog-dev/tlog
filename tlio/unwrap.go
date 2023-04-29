package tlio

type (
	Unwrapper interface {
		Unwrap() interface{}
	}
)

func Unwrap(x interface{}) interface{} {
	w, ok := x.(Unwrapper)
	if !ok {
		return nil
	}

	return w.Unwrap()
}
