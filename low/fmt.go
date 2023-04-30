package low

const Spaces = "                                                                                                                                "

type (
	Buf []byte
)

func (w *Buf) Reset() {
	*w = (*w)[:0]
}

func (w *Buf) Write(p []byte) (int, error) {
	*w = append(*w, p...)

	return len(p), nil
}

func (w *Buf) NewLine() {
	l := len(*w)
	if l == 0 || (*w)[l-1] != '\n' {
		*w = append(*w, '\n')
	}
}

func (w *Buf) Len() int      { return len(*w) }
func (w *Buf) Bytes() []byte { return *w }
