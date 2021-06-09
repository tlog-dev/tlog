package wire

import (
	"testing"

	"github.com/nikandfor/tlog/low"
)

var i interface{}

func TestEncodeValue(t *testing.T) {
	var e Encoder
	var b low.Buf

	//	v := &[4]byte{1, 2, 3, 4}
	v := &struct {
		Q [4]byte
		W []byte
	}{
		Q: [4]byte{1, 2, 3, 4},
		W: []byte{4, 3, 2, 1},
	}
	i = v
	b = e.AppendValue(b[:0], i)

	t.Logf("type %T val %[1]v is\n%s", v, Dump(b))
}
