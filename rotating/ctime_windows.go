package rotating

import (
	"io/fs"
	"syscall"
	"time"
)

func ctime(inf fs.FileInfo, now time.Time) time.Time {
	stat, ok := inf.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return now
	}

	return time.Unix(0, stat.CreationTime.Nanoseconds())
}
