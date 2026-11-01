package richtext

// publishState copies current display data without changing the document, its
// selection or its undo history. The editor's layout remains private.
func (v *view) publishState() {
	if state := v.opts.State; state != nil {
		state.Composing = v.compose != ""
		text := v.shown.String()
		if v.compose != "" {
			runes := []rune(text)
			at := min(max(v.caret, 0), len(runes))
			text = string(runes[:at]) + v.compose + string(runes[at:])
		}
		state.DisplayText = text
	}
}
