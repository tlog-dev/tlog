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

func Since(monotonic int64) time.Duration {
	return time.Duration(Monotonic() - monotonic)
}

// SplitTime is faster version of t.Date(); t.Clock().
func SplitTime(t time.Time) (year, month, day, hour, min, sec int) { //nolint:gocritic
	u := timeAbs(t)
	year, month, day, _ = absDate(u, true)
	hour, min, sec = absClock(u)
	return
}

//go:linkname timeAbs time.Time.abs
func timeAbs(time.Time) uint64

//go:linkname absClock time.absClock
func absClock(uint64) (hour, min, sec int)

//go:linkname absDate time.absDate
func absDate(uint64, bool) (year, month, day, yday int)
