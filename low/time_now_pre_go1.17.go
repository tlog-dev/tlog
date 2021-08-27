//go:build !go1.17
// +build !go1.17

package low

import (
	"time"
	_ "unsafe" // go:linkname
)

func UnixNano() int64 {
	s, n := walltime()

	return s*1e9 + int64(n)
}

//go:linkname walltime runtime.walltime1
func walltime() (sec int64, nsec int32)

//go:linkname Monotonic runtime.nanotime1

// Monotonic is runtime function. It returns monotonic nanoseconds.
func Monotonic() int64

//go:linkname MonotonicDuration runtime.nanotime1

// MonotonicDuration is runtime function. It returns monotonic time.
func MonotonicDuration() time.Duration