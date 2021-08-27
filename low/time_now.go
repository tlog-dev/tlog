//go:build go1.17

package low

import (
	"time"
	_ "unsafe"
)

//go:linkname now time.now
func now() (sec int64, nsec int32, mono int64)

func UnixNano() (t int64) {
	t, nsec, _ := now()

	t = t*1e9 + int64(nsec)

	return t
}

// Monotonic is runtime function. It returns monotonic nanoseconds.
func Monotonic() (t int64) {
	_, _, t = now()

	return t
}

//go:linkname now2 time.now
func now2() (sec int64, nsec int32, mono time.Duration)

// MonotonicDuration is runtime function. It returns monotonic time.
func MonotonicDuration() (t time.Duration) {
	_, _, t = now2()

	return t
}
