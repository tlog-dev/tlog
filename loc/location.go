package loc

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/nikandfor/tlog/low"
)

type (
	// PC is a program counter alias.
	// Function name, file name and line can be obtained from it but only in the same binary where Caller or Funcentry was called.
	PC uintptr

	// PCs is a stack trace.
	// It's quiet the same as runtime.CallerFrames but more efficient.
	PCs []PC

	locFmtState struct {
		low.Buf
		flags string
	}
)

// Caller returns information about the calling goroutine's stack. The argument s is the number of frames to ascend, with 0 identifying the caller of Caller.
//
// It's hacked version of runtime.Caller with no allocs.
func Caller(s int) (r PC) {
	caller1(1+s, &r, 1, 1)

	return
}

// Funcentry returns information about the calling goroutine's stack. The argument s is the number of frames to ascend, with 0 identifying the caller of Caller.
//
// It's hacked version of runtime.Callers -> runtime.CallersFrames -> Frames.Next -> Frame.Entry with no allocs.
func Funcentry(s int) (r PC) {
	caller1(1+s, &r, 1, 1)

	return r.Entry()
}

// Callers returns callers stack trace.
//
// It's hacked version of runtime.Callers -> runtime.CallersFrames -> Frames.Next -> Frame.Entry with only one alloc (resulting slice).
func Callers(skip, n int) PCs {
	tr := make([]PC, n)
	n = callers(1+skip, tr)
	return tr[:n]
}

// CallersFill puts callers stack trace into provided slice.
//
// It's hacked version of runtime.Callers -> runtime.CallersFrames -> Frames.Next -> Frame.Entry with no allocs.
func CallersFill(skip int, tr PCs) PCs {
	n := callers(1+skip, tr)
	return tr[:n]
}

// String formats PC as base_name.go:line.
//
// Works only in the same binary where Caller of Funcentry was called.
// Or if PC.SetCache was called.
func (l PC) String() string {
	_, file, line := l.NameFileLine()
	file = filepath.Base(file)

	b := []byte(file)
	i := len(b)
	b = append(b, ":        "...)

	n := 1
	for q := line; q != 0; q /= 10 {
		n++
	}

	b = b[:i+n]

	for q, j := line, n-1; j >= 1; j-- {
		b[i+j] = byte(q%10) + '0'
		q /= 10
	}

	return string(b)
}

// Format is fmt.Formatter interface implementation.
// It supports width. Precision sets line number width. '+' prints full path not base.
func (l PC) Format(s fmt.State, c rune) {
	name, file, line := l.NameFileLine()

	nn := file

	if s.Flag('#') {
		nn = name
	}

	if !s.Flag('+') {
		nn = filepath.Base(nn)
		if s.Flag('#') && !s.Flag('-') {
			p := strings.IndexByte(nn, '.')
			nn = nn[p+1:]
		}
	}

	n := 1
	for q := line; q != 0; q /= 10 {
		n++
	}

	p, ok := s.Precision()

	if ok {
		n = 1 + p
	}

	s.Write([]byte(nn))

	w, ok := s.Width()

	if ok {
		p := w - len(nn) - n
		if p > 0 {
			s.Write(low.Spaces[:p])
		}
	}

	var b [20]byte
	copy(b[:], ":                  ")

	for q, j := line, n-1; q != 0 && j >= 1; j-- {
		b[j] = byte(q%10) + '0'
		q /= 10
	}

	s.Write(b[:n])
}

// String formats PCs as list of type_name (file.go:line)
//
// Works only in the same binary where Caller of Funcentry was called.
// Or if PC.SetCache was called.
func (t PCs) String() string {
	var b []byte
	for _, l := range t {
		n, f, l := l.NameFileLine()
		n = path.Base(n)
		b = low.AppendPrintf(b, "%-60s  at %s:%d\n", n, f, l)
	}
	return string(b)
}

// StringFlags formats PCs as list of type_name (file.go:line)
//
// Works only in the same binary where Caller of Funcentry was called.
// Or if PC.SetCache was called.
func (t PCs) FormatString(flags string) string {
	s := locFmtState{flags: flags}

	t.Format(&s, 'v')

	return string(s.Buf)
}

func (t PCs) Format(s fmt.State, c rune) {
	switch {
	case s.Flag('+'):
		for _, l := range t {
			s.Write([]byte("at "))
			l.Format(s, c)
			s.Write([]byte("\n"))
		}
	default:
		for i, l := range t {
			if i != 0 {
				s.Write([]byte(" at "))
			}
			l.Format(s, c)
		}
	}
}

func cropFilename(fn, tp string) string {
	p := strings.LastIndexByte(tp, '/')
	pp := strings.IndexByte(tp[p+1:], '.')
	tp = tp[:p+pp]

again:
	if p = strings.Index(fn, tp); p != -1 {
		return fn[p:]
	}

	p = strings.IndexByte(tp, '/')
	if p == -1 {
		return filepath.Base(fn)
	}
	tp = tp[p+1:]
	goto again
}

func (s *locFmtState) Flag(c int) bool {
	for _, f := range s.flags {
		if f == rune(c) {
			return true
		}
	}

	return false
}

func (s *locFmtState) Width() (int, bool)     { return 0, false }
func (s *locFmtState) Precision() (int, bool) { return 0, false }
