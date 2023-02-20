package tlio

import (
	"fmt"
	"strings"
)

func (w MultiWriter) String() string {
	var b strings.Builder

	_, _ = b.WriteString("MultiWriter{")

	for i, w := range w {
		if i != 0 {
			_, _ = b.WriteString(" ")
		}

		writeWriter(&b, w)
	}

	_, _ = b.WriteString("}")

	return b.String()
}

func (w WriteCloser) String() string {
	var b strings.Builder

	_, _ = b.WriteString("WriteCloser{")

	_, _ = b.WriteString("Writer:")
	writeWriter(&b, w.Writer)

	_, _ = b.WriteString(" Closer:")
	writeWriter(&b, w.Closer)

	_, _ = b.WriteString("}")

	return b.String()
}

func writeWriter(b *strings.Builder, o interface{}) {
	if s, ok := o.(fmt.Stringer); ok {
		_, _ = b.WriteString(s.String())
	} else {
		_, _ = fmt.Fprintf(b, "%T", o)
	}
}
