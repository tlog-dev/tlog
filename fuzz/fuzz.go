package fuzz

import (
	"bytes"
	"io"

	"github.com/nikandfor/tlog/parse"
)

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

func FuzzProto(d []byte) int {
	r := parse.NewProtoReader(bytes.NewReader(d))

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
