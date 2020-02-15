package fuzz

import (
	"bytes"
	"io"

	"github.com/nikandfor/tlog/parse"
)

//nolint:golint
func FuzzJSON(d []byte) int {
	r := parse.NewJSONReader(bytes.NewReader(d))

	for {
		_, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0
		}
	}

	return 1
}

//nolint:golint
func FuzzProto(d []byte) int {
	r := parse.NewProtoReader(bytes.NewReader(d))
	r.MaxRecordLen = 1 << 20

	for {
		_, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0
		}
	}

	return 1
}
