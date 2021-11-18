package wire

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/nikandfor/tlog/low"
)

func TestDumper(t *testing.T) {
	t.Parallel()

	var (
		b, db low.Buf
		e     Encoder
		d     = Dumper{Writer: &db}
	)

	defer func() {
		p := recover()
		if p == nil {
			return
		}

		t.Logf("written:\n%s", d.b)
		t.Logf("dump:\n%s", hex.Dump(b))

		panic(p)
	}()

	b = e.AppendMap(b, -1)

	b = e.AppendKeyString(b, "key_a", "val_a")

	b = e.AppendKeyInt(b, "key_b", 2)

	b = e.AppendString(b, "key_c")
	b = e.AppendDuration(b, time.Second)

	b = e.AppendBreak(b)

	_, err := d.Write(b)
	assert.NoError(t, err)

	_, err = d.Write(b)
	assert.NoError(t, err)

	assert.Equal(t, `       0     0  bf  -  map: len -1
       1     1    65  -  "key_a"
       7     7    65  -  "val_a"
       d     d    65  -  "key_b"
      13    13    02  -  int          2
      14    14    65  -  "key_c"
      1a    1a    c3  -  semantic  3
      1b    1b      1a 3b 9a ca 00  -  int 1000000000
      20    20    ff  -  break
      21     0  bf  -  map: len -1
      22     1    65  -  "key_a"
      28     7    65  -  "val_a"
      2e     d    65  -  "key_b"
      34    13    02  -  int          2
      35    14    65  -  "key_c"
      3b    1a    c3  -  semantic  3
      3c    1b      1a 3b 9a ca 00  -  int 1000000000
      41    20    ff  -  break
`, string(db))
}
