package compress

import (
	"testing"
	"unsafe"

	"github.com/nikandfor/tlog/low"
)

func BenchmarkMemHash(b *testing.B) {
	data := []byte("01234567")

	b.Run("32", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			low.MemHash32(unsafe.Pointer(&data[0]), 0)
		}
	})

	b.Run("64", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			low.MemHash64(unsafe.Pointer(&data[0]), 0)
		}
	})

	b.Run("N-64", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			low.MemHash(unsafe.Pointer(&data[0]), 0, uintptr(len(data)))
		}
	})
}
