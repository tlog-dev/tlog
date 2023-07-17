package rotating

import (
	"io/fs"
	"syscall"
	"time"
)

func fileCtime(fstat func(string) (fs.FileInfo, error), name string, now time.Time) time.Time {
	inf, err := fstat(name)
	if err != nil {
		return now
	}

	return ctime(inf, now)
}

func ctime(inf fs.FileInfo, now time.Time) time.Time {
	stat, ok := inf.Sys().(*syscall.Stat_t)
	if !ok {
		return now
	}

	return time.Unix(stat.Ctimespec.Unix())
}
