package desktop

import "github.com/egoist/mygo/ui"

type settingsInputFocus struct {
	focused bool
	dirty   bool
	initial string
}

// settingsInputCommit owns focus tracking only; callers own persistence.
func settingsInputCommit(input ui.Element, value *string, commit func()) ui.Element {
	state := ui.Local(input, "settings-input-focus", func() settingsInputFocus {
		return settingsInputFocus{initial: *value}
	})
	focused := input.FocusWithin()
	if focused && !state.focused {
		state.initial = *value
	}
	changed := state.dirty && !focused && state.initial != *value
	state.focused = focused
	if changed {
		state.initial = *value
		state.dirty = false
		commit()
	}
	return input.OnChange(func() { state.dirty = true })
}
