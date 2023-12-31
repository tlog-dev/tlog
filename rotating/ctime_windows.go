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
	stat, ok := inf.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return now
	}

	return time.Unix(0, stat.CreationTime.Nanoseconds())
}
