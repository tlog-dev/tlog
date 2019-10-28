package tlog

import (
	"fmt"
	"os"
	"testing"
)

func TestRotatedFile(t *testing.T) {
	f := NewFile(fmt.Sprintf("/tmp/tlog_test_#.%d.log", os.Getpid()))
	defer f.Close()
	f.MaxSize = 20

	l := New(NewConsoleWriter(f, LstdFlags))

	l.Printf("some info %v %v", os.Args, 1)
	l.Printf("some info %v %v", os.Args, 2)
	l.Printf("some info %v %v", os.Args, 3)
}
