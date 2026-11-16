package redact

// TextStream restores decoded text fragments without exposing a partial
// placeholder. Keep one per output channel and per physical request attempt.
// This adapter is GCode's; the recognition and restoration rules are magpie's.
type TextStream struct{ tail string }

func (s *TextStream) Write(text string) string {
	text = s.tail + text
	n := partialTail(text)
	s.tail = text[len(text)-n:]
	return Restore(text[:len(text)-n], false)
}

func (s *TextStream) Finish() string {
	text := Restore(s.tail, false)
	s.tail = ""
	return text
}
