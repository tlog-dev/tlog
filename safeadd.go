package tlog

import "unicode/utf8"

const tohex = "0123456789abcdef"

// appendSafe appends string to buffer with JSON compatible esaping
func appendSafe(b []byte, s string) []byte {
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
	case '"':
		b = append(b, '\\', '"')
	case '\\':
		b = append(b, '\\', '\\')
	default:
		goto complex
	}

	s = s[i+1:]

	goto again

complex:
	r, width := utf8.DecodeRuneInString(s)

	switch {
	case r == utf8.RuneError && width == 1:
		b = append(b, '\\', 'x', tohex[s[0]>>4], tohex[s[0]&0xf])
	case r <= 0xffff:
		b = append(b, '\\', 'u', tohex[r>>12&0xf], tohex[r>>8&0xf], tohex[r>>4&0xf], tohex[r&0xf])
	default:
		b = append(b, '\\', 'U', tohex[r>>28&0xf], tohex[r>>24&0xf], tohex[r>>20&0xf], tohex[r>>16&0xf], tohex[r>>12&0xf], tohex[r>>8&0xf], tohex[r>>4&0xf], tohex[r&0xf])
	}

	s = s[width:]

	goto again
}
