// +build ignore
// +build linux darwin

package rotated

import (
	"os"
	"syscall"
)

func CreateLogrotate(name string, sig ...os.Signal) *File {
	f := &File{
		name:     name,
		MaxSize:  0,
		Fallback: os.Stderr,
		Mode:     0640,
		Fopen:    FopenSimple,
		stopc:    make(chan struct{}),
	}

	if len(sig) == 0 {
		sig = append(sig, syscall.SIGUSR1)
	}

	f.RotateOnSignal(sig...)

	return f
}
