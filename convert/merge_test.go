package convert

import (
	"testing"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/low"
)

func TestSetAdd(t *testing.T) {
	msg := encode("key", "val", "key2", 1)

	L := encode("L", tlog.Labels{"a=b", "c"})
	L = L[1:] // cut Map

	res := Set(nil, msg, L)

	t.Logf("sum:\n%v", tlog.Dump(res))
}

func TestSetReplace(t *testing.T) {
	msg := encode("key", "val", "L", tlog.Labels{"replace"}, "key2", 1)

	L := encode("L", tlog.Labels{"a=b", "c"})
	L = L[1:] // cut Map

	res := Set(nil, msg, L)

	t.Logf("sum:\n%v", tlog.Dump(res))
}

func encode(kvs ...interface{}) []byte {
	var msg low.Buf
	e := tlog.Encoder{Writer: &msg}

	e.Encode(nil, kvs)

	return msg
}
