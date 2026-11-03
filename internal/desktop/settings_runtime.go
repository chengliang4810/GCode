package desktop

import (
	"crypto/rand"
	"slices"
	"strconv"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/model"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type runtimeSettingsState struct {
	connection                                contracts.ConnectionInfo
	remoteURL, remoteToken                    string
	connectionSaving                          bool
	directory, directorySource                string
	provider                                  model.Provider
	key                                       string
	subagentOpen                              bool
	subagent                                  contracts.SubagentProfile
	subagentExisting                          bool
	subagentModel, subagentReasoning          string
	subagentToolsCustom                       bool
	subagentFieldErrors                       map[string]string
	subagentError                             string
	subagentPendingID, subagentPendingProfile string
	subagentCloseAfterSave                    bool
}

type runtimeModelDraft struct {
	key             string
	model           model.Model
	context, output string
	lookup          uint64
	loading         bool
}

var configurableReasoningLevels = []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"}

func draftRuntimeModel(m model.Model) runtimeModelDraft {
	m.InputTypes = append([]string(nil), m.InputTypes...)
	if !slices.Contains(m.InputTypes, "text") {
		m.InputTypes = append([]string{"text"}, m.InputTypes...)
	}
	m.ReasoningLevels = append([]string(nil), m.ReasoningLevels...)
	return runtimeModelDraft{key: rand.Text(), model: m, context: strconv.Itoa(m.ContextWindow), output: strconv.Itoa(m.MaxOutput)}
}

func (a *App) dataDirectorySettings(c *ui.Context) {
	info := a.state.Runtime
	if info == nil {
		return
	}
	state := &a.runtimeUI
	source := info.DataDirectory + "\n" + info.PendingDirectory
	if state.directorySource != source {
		state.directorySource = source
		state.directory = info.DataDirectory
		if info.PendingDirectory != "" {
			state.directory = info.PendingDirectory
		}
	}
	a.settingsCard(c, func() {
		current := ""
		if info.PendingDirectory != "" {
			current = info.DataDirectory
		}
		a.settingsRow(c, "Data directory", current, func() {})
		ui.Column(c).Padding(0, 16, 12, 16).Gap(8).Children(func() {
			input := controlTextInput(c.Key("data-directory"), &state.directory).Label(tr(c, "Data directory")).FillWidth()
			settingsInputCommit(input, &state.directory, func() {
				a.dispatch(contracts.Command{Kind: "storage.change", Path: state.directory})
			})
			ui.Row(c).Gap(8).Children(func() {
				textButton(c.Key("data-directory-browse"), tr(c, "Browse…")).Disabled(info.Remote).OnClick(func() {
					window := a.win
					if window == nil {
						return
					}
					go func() {
						paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: window, Directory: true, CreateDirectories: true})
						window.Update(func() {
							if a.closed {
								return
							}
							if err != nil {
								a.errorText = err.Error()
							} else if len(paths) > 0 {
								a.runtimeUI.directory = paths[0]
								a.dispatch(contracts.Command{Kind: "storage.change", Path: paths[0]})
							}
						})
					}()
				})
			})
			if info.PendingDirectory != "" {
				ui.Text(c, tr(c, "The new directory takes effect after restart. Existing conversations stay in their original directories; no data is copied.")).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted)
			}
			if info.StorageError != "" {
				ui.Text(c, tr(c, "Data directory change failed; the current directory is still active.")).TextColor(a.palette.Danger)
				ui.Text(c, info.StorageError).Selectable().TextColor(a.palette.Muted)
			}
			if info.Remote {
				ui.Text(c, tr(c, "This directory belongs to the remote host. Restart the host to apply changes.")).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted)
			}
		})
	})
}
