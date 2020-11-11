package tlog

import (
	"time"
	_ "unsafe" // go:linkname
)

func fastnow() int64 {
	s, n := walltime()

	return s*1e9 + int64(n)
}

//go:linkname walltime runtime.walltime1
func walltime() (sec int64, nsec int32)

// monotonic clock
//go:linkname nanotime runtime.nanotime1
func nanotime() int64

func splitTime(t time.Time) (year, month, day, hour, min, sec int) {
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
