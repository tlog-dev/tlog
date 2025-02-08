package rotating

import (
	"io/fs"
	"syscall"
	"time"
)

func ctime(inf fs.FileInfo, now time.Time) time.Time {
	stat, ok := inf.Sys().(*syscall.Stat_t)
	if !ok {
		return now
	}

	return time.Unix(stat.Ctim.Unix())
}
