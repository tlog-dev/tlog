//nolint
package tlog

import (
	"reflect"
	"unsafe"
	_ "unsafe"
)

type (
	// Use simple []byte instead of bytes.Buffer to avoid large dependency.
	buffer []byte

	// pp is used to store a printer's state and is reused with sync.Pool to avoid allocations.
	pp struct {
		buf []byte

		// arg holds the current item, as an interface{}.
		arg interface{}

		// value is used instead of arg for reflect values.
		value reflect.Value

		// fmt is used to format basic items such as integers or strings.
		fmt fmtt

		// reordered records whether the format string used argument reordering.
		reordered bool
		// goodArgNum records whether the most recent reordering directive was valid.
		goodArgNum bool
		// panicking is set by catchPanic to avoid infinite panic, recover, panic, ... recursion.
		panicking bool
		// erroring is set when printing an error string to guard against calling handleMethods.
		erroring bool
		// wrapErrs is set when the format string may contain a %w verb.
		wrapErrs bool
		// wrappedErr records the target of the %w verb.
		wrappedErr error
	}

	// A fmt is the raw formatter used by Printf etc.
	// It prints into a buffer that must be set up separately.
	fmtt struct {
		buf *[]byte

		fmtFlags

		wid  int // width
		prec int // precision

		// intbuf is large enough to store %b of an int64 with a sign and
		// avoids padding at the end of the struct on 32 bit architectures.
		intbuf [68]byte
	}

	// flags placed in a separate struct for easy clearing.
	fmtFlags struct {
		widPresent  bool
		precPresent bool
		minus       bool
		plus        bool
		sharp       bool
		space       bool
		zero        bool

		// For the formats %+v %#v, we set the plusV/sharpV flags
		// and clear the plus/sharp flags since %+v and %#v are in effect
		// different, flagless formats set at the top level.
		plusV  bool
		sharpV bool
	}
)

//go:linkname doPrintf fmt.(*pp).doPrintf
//go:noescape
func doPrintf(p *pp, format string, a []interface{})

// AppendPrintf is similar to fmt.Fprintf but a little bit hacked.
//
// There is no sync.Pool.Get and Put. There is no copying buffer to io.Writer or conversion to string. There is no io.Writer interface dereference.
// All that gives advantage about 30-50 ns per call. Yes, I know :).
func AppendPrintf(b []byte, format string, a ...interface{}) []byte {
	var p pp
	p.buf = b
	p.fmt.buf = &p.buf
	doPrintf(&p, format, a)
	b = *(*[]byte)(noescape(unsafe.Pointer(&p.buf)))
	return b
	//	return p.buf
}

// noescape hides a pointer from escape analysis.  noescape is
// the identity function but escape analysis doesn't think the
// output depends on the input.  noescape is inlined and currently
// compiles down to zero instructions.
// USE CAREFULLY!
//go:nosplit
func noescape(p unsafe.Pointer) unsafe.Pointer {
	x := uintptr(p)
	return unsafe.Pointer(x ^ 0) //nolint:staticcheck
}
