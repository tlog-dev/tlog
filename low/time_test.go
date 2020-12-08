package low

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTimeFastNow(t *testing.T) {
	now := time.Now()
	ts := UnixNano()

	tsnow := time.Unix(0, ts)

	diff := tsnow.Sub(now)

	assert.True(t, diff < time.Millisecond, "got %v  wanted %v", tsnow, now)
}

func BenchmarkTimeNow(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = time.Now().UnixNano()
	}
}

func BenchmarkTimeFastNow(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = UnixNano()
	}
}
