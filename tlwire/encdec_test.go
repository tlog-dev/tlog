package tlwire

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAddr(tb *testing.T) {
	var e Encoder
	var d Decoder
	var b []byte

	for _, a := range []string{"1.1.1.1", "ff22::ff11"} {
		x := netip.MustParseAddr(a)

		b = e.AppendAddr(b[:0], x)

		y, z, j, err := d.Addr(b, 0)
		assert.NoError(tb, err)
		assert.Equal(tb, len(b), j)
		assert.Equal(tb, x, y)
		assert.False(tb, z.IsValid())
	}

	for _, a := range []string{"1.1.1.1:8080", "[ff22::ff11]:1234"} {
		x := netip.MustParseAddrPort(a)

		b = e.AppendAddrPort(b[:0], x)

		y, z, j, err := d.Addr(b, 0)
		assert.NoError(tb, err)
		assert.Equal(tb, len(b), j)
		assert.Equal(tb, x, z)
		assert.False(tb, y.IsValid())
	}

	b[0] = Semantic | NetAddr
	b = e.AppendString(b[:1], "qweqwe")

	_, _, _, err := d.Addr(b, 0)
	assert.Error(tb, err)
}
