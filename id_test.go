package tlog

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"sync"
	"testing"

	"github.com/nikandfor/assert"
)

func testRandID(seed int64) func() ID {
	var mu sync.Mutex
	rnd := rand.New(rand.NewSource(seed)) //nolint:gosec

	return func() (id ID) {
		defer mu.Unlock()
		mu.Lock()

		for id == (ID{}) {
			_, _ = rnd.Read(id[:])
		}

		return
	}
}

func TestIDFromString(tb *testing.T) {
	id, err := IDFromString("e6a5d996-99b1-493e-ad74-47382220d1a9")
	assert.NoError(tb, err)
	assert.Equal(tb, ID{0xe6, 0xa5, 0xd9, 0x96, 0x99, 0xb1, 0x49, 0x3e, 0xad, 0x74, 0x47, 0x38, 0x22, 0x20, 0xd1, 0xa9}, id)

	_, err = IDFromString("e6a5d996-99b1-493e-ad74-47382220d1a")
	assert.ErrorIs(tb, err, ShortIDError{Bytes: 15})
}

func TestIDJSON(t *testing.T) {
	id := testRandID(1)()

	data, err := json.Marshal(id)
	assert.NoError(t, err)

	t.Logf("json encoded id: %s", data)

	var back ID
	err = json.Unmarshal(data, &back)
	assert.NoError(t, err)

	assert.Equal(t, id, back)
}

func BenchmarkIDFormat(b *testing.B) {
	b.ReportAllocs()

	id := ID{1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 0xa, 0xb, 0xc, 0xd, 0xe, 0xf}

	for i := 0; i < b.N; i++ {
		fmt.Fprintf(ioutil.Discard, "%+x", id)
	}
}

func BenchmarkIDFormatTo(b *testing.B) {
	b.ReportAllocs()

	var buf [40]byte
	id := ID{1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 0xa, 0xb, 0xc, 0xd, 0xe, 0xf}

	for i := 0; i < b.N; i++ {
		if i&0xf == 0 {
			ID{}.FormatTo(buf[:], 'v')
		} else {
			id.FormatTo(buf[:], 'v')
		}
	}
}
