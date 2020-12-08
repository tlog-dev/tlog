package writer

type (
	JSON struct {
		// Key names
		Time,
		Type,
		Level,
		PC,
		Labels,
		Span,
		Parent,
		Elapsed,
		Message,
		Value,
		UserTags,
		UserFields string
	}
)

func (w *JSON) Write(p []byte) (int, error) {
	panic("nope")
}
