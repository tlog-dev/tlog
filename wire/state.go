package wire

import (
	"fmt"
	"io"

	"github.com/nikandfor/errors"
)

type (
	State struct {
		io.Reader

		s  []s
		ss *SubState

		b []byte

		ref int64
	}

	SubState struct {
		Tag byte
		Sub int64

		d int

		s  *State
		p  *SubState
		ss *SubState
	}

	s interface{}

	num struct{}

	str int

	arr struct {
		i, n int
		s    byte
	}

	obj struct {
		i, n int
		s    byte
	}

	sem byte

	spl struct{}
)

var ErrSubstate = errors.New("in substate")

func (s *State) top() (x s) {
	l := len(s.s)
	if l == 0 {
		return nil
	}

	return s.s[l-1]
}

func (s *State) pop() (x s) {
	l := len(s.s)
	if l == 0 {
		return nil
	}

	l--

	x = s.s[l]
	s.s = s.s[:l]

	return
}

func (s *State) push(x s) {
	s.s = append(s.s, x)
}

func (s *State) Read(p []byte) (i int, err error) {
	if s.ss != nil {
		return s.ss.Read(p)
	}

	return s.read(p, 0)
}

func (s *State) read(p []byte, lvl int) (i int, err error) {
	n := copy(p, s.b)

	defer func() {
		s.b = append(s.b[:0], p[i:n]...)
	}()

	if n != len(p) {
		var m int

		if s.Reader == nil {
			return 0, io.EOF
		}

		m, err = s.Reader.Read(p[n:])
		n += m
		s.ref += int64(m)
		if err != nil {
			return 0, err
		}
	}

	i, err = s.skip(p[:n], 0, lvl)

	return
}

func (s *State) SubState(p []byte) (sub *SubState, i int, err error) {
	if s.ss != nil {
		return s.ss, 0, nil
	}

	n := copy(p, s.b)

	defer func() {
		s.b = append(s.b[:0], p[i:n]...)
	}()

	if n != len(p) {
		var m int

		if s.Reader == nil {
			return nil, 0, io.EOF
		}

		m, err = s.Reader.Read(p[n:])
		n += m
		s.ref += int64(m)
		if err != nil {
			return
		}
	}

	tag, stag, i, err := s.headVal(p[:n], 0)
	if err != nil {
		return nil, 0, err
	}

	sub = &SubState{
		Tag: tag,
		Sub: stag,
		d:   1,
		s:   s,
	}

	s.ss = sub

	return sub, i, nil
}

func (ss *SubState) Read(p []byte) (int, error) {
	if ss.ss != nil {
		return 0, errors.New("in substate")
	}

	if ss.s == nil {
		return 0, io.EOF
	}

	return ss.s.read(p, ss.d)
}

func (ss *SubState) Close() error {
	if ss.s == nil {
		return nil
	}

	ss.s.ss = nil
	ss.s = nil

	return nil
}

func (s *State) skip(p []byte, st, lvl int) (i int, err error) {
	for {
		if len(s.s) < lvl {
			return i, io.EOF
		}

		//	println(fmt.Sprintf("skip(%v) st %x/%x => %x  x %v", lvl, st, len(p), i, s.s))

		switch x := s.top().(type) {
		case nil:
			i, err = s.skipVal(p, i, lvl)
		case num:
			i, err = s.skipInt(p, i)
		case *str:
			i, err = s.skipStr(p, i, x)
		case *arr:
			i, err = s.skipArr(p, i, x, lvl)
		case *obj:
			i, err = s.skipObj(p, i, x, lvl)
		case *sem:
			i, err = s.skipSem(p, i, x, lvl)
		case spl:
			i, err = s.skipSpecial(p, i)
		default:
			panic(x)
		}

		if err != nil {
			return
		}

		if len(s.s) == lvl {
			return
		}
	}
}

func (s *State) headVal(p []byte, st int) (tag byte, sub int64, i int, err error) {
	tag, sub, i = readTag(p, st)
	if i < 0 {
		i, err = 0, stateError(i)
		return
	}

	switch tag {
	case Int, Neg:
		s.push(num{})

		i = st
	case String, Bytes:
		v := str(sub)
		x := &v
		s.push(x)

		i = st
	case Array:
		x := &arr{n: int(sub)}
		s.push(x)
	case Map:
		x := &obj{n: int(sub)}
		s.push(x)
	case Semantic:
		v := sem(0)
		x := &v
		s.push(x)
	case Special:
		s.push(spl{})

		i = st
	default:
		panic(tag)
	}

	return
}

func (s *State) skipVal(p []byte, st, lvl int) (i int, err error) {
	tag, sub, i := readTag(p, st)

	switch tag {
	case Int, Neg:
		if i < 0 {
			s.push(num{})

			return st, stateError(i)
		}

		return i, nil
	case String, Bytes:
		v := str(sub)
		x := &v
		s.push(x)

		return s.skipStr(p, i, x)
	case Array:
		x := &arr{n: int(sub)}
		s.push(x)

		return s.skipArr(p, i, x, lvl)
	case Map:
		x := &obj{n: int(sub)}
		s.push(x)

		return s.skipObj(p, i, x, lvl)
	case Semantic:
		v := sem(0)
		x := &v
		s.push(x)

		return s.skipSem(p, st, x, lvl)
	case Special:
		switch sub {
		case False,
			True,
			Nil,
			Undefined,
			Break:
		case Float8, Float16, Float32, Float64:
			w := 1 << (sub - Float8)

			if i+w > len(p) {
				s.push(spl{})

				return st, io.ErrShortBuffer
			}

			i += w
		default:
			return st, errors.New("unsupported special")
		}

		return
	}

	panic(tag)
}

func (s *State) skipInt(p []byte, st int) (i int, err error) {
	//	defer func() {
	//		println("skipInt", fmt.Sprintf("st %x p %x  =>  %x %v", st, len(p), i, err))
	//	}()

	_, _, i = readTag(p, st)
	if i < 0 {
		return st, stateError(i)
	}

	s.pop()

	return i, nil
}

func (s *State) skipStr(p []byte, i int, x *str) (_ int, err error) {
	//	defer func() {
	//		println("skipStr", fmt.Sprintf("st %x p %x x %x  =>  %x %v", st, len(p), x, s.top(), err))
	//	}()

	if i+int(*x) <= len(p) {
		s.pop()

		return i + int(*x), nil
	}

	*x -= str(len(p) - i)

	return len(p), io.ErrShortBuffer
}

func (s *State) skipArr(p []byte, st int, x *arr, lvl int) (i int, err error) {
	//	defer func(x0 arr) {
	//		println(fmt.Sprintf("skipArr st %x/%x => %x %v  x %v <- %v  p % x %[7]q", st, len(p), i, err, x, x0, p[st:i]))
	//	}(*x)

	i = st

	for ; x.n == -1 || x.i < x.n; x.i++ {
		if i == len(p) && x.s != 'v' {
			return i, io.ErrShortBuffer
		}

		if x.s == 0 {
			if x.n == -1 && p[i] == Special|Break {
				i++
				break
			}

			x.s = 'v'

			i, err = s.skipVal(p, i, lvl)
			if err != nil {
				return
			}

			if len(s.s) == lvl {
				return
			}
		}

		if x.s == 'v' {
			x.s = 0
		}
	}

	s.pop()

	return
}

func (s *State) skipObj(p []byte, st int, x *obj, lvl int) (i int, err error) {
	//	defer func(x0 obj) {
	//		println(fmt.Sprintf("skipObj st %x/%x => %x %v  x %v <- %v  p % x %[7]q", st, len(p), i, err, x, x0, p[st:i]))
	//	}(*x)

	i = st

	for ; x.n == -1 || x.i < x.n; x.i++ {
		if i == len(p) && x.s != 'v' {
			return i, io.ErrShortBuffer
		}

		//	fmt.Fprintf(os.Stderr, "here0 %c i %x\n", x.s, i)

		if x.s == 0 {
			if x.n == -1 && p[i] == Special|Break {
				i++
				break
			}

			x.s = 'k'

			i, err = s.skipVal(p, i, lvl)
			if err != nil {
				return
			}
		}

		//	fmt.Fprintf(os.Stderr, "here1 %c\n", x.s)

		if x.s == 'k' {
			x.s = 'v'

			i, err = s.skipVal(p, i, lvl)
			if err != nil {
				return
			}

			if len(s.s) == lvl {
				return
			}
		}

		//	fmt.Fprintf(os.Stderr, "here2 %c\n", x.s)

		if x.s == 'v' {
			x.s = 0
		}

		//	fmt.Fprintf(os.Stderr, "here3 %c\n", x.s)
	}

	s.pop()

	return
}

func (s *State) skipSem(p []byte, st int, x *sem, lvl int) (i int, err error) {
	//	defer func(x0 sem) {
	//		println("skipSem", fmt.Sprintf("st %x p %x  =>  %x %v  %v <- %v  from %v", st, len(p), i, err, x, x0, loc.Caller(1)))
	//	}(*x)

	i = st

	if *x == 0 {
		_, _, i = readTag(p, st)

		if i < 0 {
			return st, stateError(i)
		}

		*x = 'v'

		i, err = s.skipVal(p, i, lvl)
		if err != nil {
			return
		}
	}

	if *x == 'v' {
		s.pop()
	}

	return i, nil
}

func (s *State) skipSpecial(p []byte, st int) (i int, err error) {
	_, sub, i := readTag(p, st)
	if i < 0 {
		return st, stateError(i)
	}

	switch sub {
	case Float8, Float16, Float32, Float64:
		w := 1 << (sub - Float8)

		if i+w > len(p) {
			return st, io.ErrShortBuffer
		}

		i += w
	default:
		return st, errors.New("unsupported special")
	}

	s.pop()

	return
}

func stateError(i int) error {
	if i >= 0 {
		return nil
	}

	switch i {
	case eUnexpectedEOF:
		return io.ErrShortBuffer
	case eBadFormat:
		return errors.New("bad format")
	case eBadSpecial:
		return errors.New("unsupported special")
	default:
		panic(i)
	}
}

func (x num) String() string {
	return fmt.Sprintf("num")
}

func (x str) String() string {
	return fmt.Sprintf("str(%d)", int(x))
}

func (x arr) String() string {
	return fmt.Sprintf("arr{%d,%d,%c}", x.i, x.n, x.s)
}

func (x obj) String() string {
	return fmt.Sprintf("map{%d,%d,%c}", x.i, x.n, x.s)
}

func (x sem) String() string {
	return fmt.Sprintf("sem(%c)", byte(x))
}

func (x spl) String() string {
	return fmt.Sprintf("spl")
}
