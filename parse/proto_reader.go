package parse

import (
	"fmt"
	"io"
)

type ProtoReader struct {
	r   io.Reader
	buf []byte
	st  int
}

func NewProtoReader(r io.Reader) *ProtoReader {
	return &ProtoReader{r: r}
}

func (r *ProtoReader) Read() (interface{}, error) {
	l, err := r.varint() // record len
	if err != nil {
		return nil, err
	}
	_ = l
	tag, err := r.varint()
	if err != nil {
		return nil, err
	}

	switch tag >> 3 {
	case 1:
		return r.labels()
	case 2:
		return r.location()
	case 3:
		return r.message()
	case 4:
		return r.spanStart()
	case 5:
		return r.spanFinish()
	default:
		return nil, fmt.Errorf("unexpected object %x", tag)
	}
}

func (r *ProtoReader) labels() (interface{}, error) {
	return nil, nil
}

func (r *ProtoReader) location() (interface{}, error) {
	return nil, nil
}

func (r *ProtoReader) message() (interface{}, error) {
	return nil, nil
}

func (r *ProtoReader) spanStart() (interface{}, error) {
	return nil, nil
}

func (r *ProtoReader) spanFinish() (interface{}, error) {
	return nil, nil
}

func (r *ProtoReader) varint() (int64, error) {
	return 0, nil
}

func (r *ProtoReader) more() error {
	return nil
}
