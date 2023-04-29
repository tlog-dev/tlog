package tlog

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"sync"
	"testing"
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
