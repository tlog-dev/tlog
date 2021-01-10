package low

import "unicode/utf8"

const tohex = "0123456789abcdef"

// AppendSafe appends string to buffer with JSON compatible esaping.
// It does NOT add quotes.
func AppendSafe(b []byte, s string) []byte {
again:
	i := 0
	l := len(s)
	for i < l {
		c := s[i]
		if c == '"' || c == '\\' || c < 0x20 || c > 0x7e {
			break
		}
		i++
	}
	b = append(b, s[:i]...)
	if i == l {
		return b
	}

	switch s[i] {
	case '"', '\\':
		b = append(b, '\\', s[i])
	case '\n':
		b = append(b, '\\', 'n')
	case '\t':
		b = append(b, '\\', 't')
	case '\v':
		b = append(b, '\\', 'v')
	case '\r':
		b = append(b, '\\', 'r')
	case '\a':
		b = append(b, '\\', 'a')
	case '\b':
		b = append(b, '\\', 'b')
	case '\f':
		b = append(b, '\\', 'f')
	default:
		goto complex
	}

	s = s[i+1:]

	goto again

complex:
	r, width := utf8.DecodeRuneInString(s[i:])

	if r == utf8.RuneError && width == 1 {
		b = append(b, '\\', 'x', tohex[s[i]>>4], tohex[s[i]&0xf])
	} else {
		b = append(b, s[i:i+width]...)
	}

	s = s[i+width:]

	goto again
}

func AppendQuote(b []byte, s string) []byte {
	b = append(b, '"')
	b = AppendSafe(b, s)
	b = append(b, '"')

	return b
}
