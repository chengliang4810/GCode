package desktop

import (
	"fmt"
	"slices"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

type settingsRemovalState struct {
	Open           bool
	Kind, ID, Name string
}

func (a *App) settingsDialog(c *ui.Context, key, title string, open *bool, width float32, draw func()) {
	a.settingsDialogWithFooter(c, key, title, open, width, draw, nil)
}

func (a *App) settingsDialogWithFooter(c *ui.Context, key, title string, open *bool, width float32, draw, footer func()) {
	w, h := c.Size()
	ui.DialogBase(c.Key(key), open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, .45))
		panel.Width(min(width, w-40)).MaxHeight(h - 40).Padding(20).Gap(16).Radius(12).Label(tr(c, title))
		a.floatingSurface(c, panel, 12)
		ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, tr(c, title)).Grow(1).MinWidth(0).FontSize(design.TextSize(c, design.TextXL)).FontWeight(600)
			iconButton(c.Key("dialog-close"), "close", tr(c, "Close dialog")).OnClick(func() { *open = false })
		})
		reserve := float32(160)
		if footer != nil {
			reserve += 48
		}
		ui.Scroll(c.Key("dialog-content")).FillWidth().MaxHeight(max(float32(120), h-reserve)).Gap(12).
			Children(draw)
		if footer != nil {
			footer()
		}
	})
}

func (a *App) settingsFormHeader(c *ui.Context, section, title string, close func()) {
	ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		textButton(c, tr(c, section)).Label(tr(c, "Back to list")).OnClick(close)
		ui.Text(c, "›").TextColor(a.palette.Muted)
		ui.Text(c, tr(c, title)).MinWidth(0).SingleLine()
	})
}

func (a *App) confirmSettingsRemoval(kind, id, name string) {
	a.settingsRemoval = settingsRemovalState{Open: true, Kind: kind, ID: id, Name: name}
}

func (a *App) settingsRemovalDialog(c *ui.Context) {
	state := &a.settingsRemoval
	a.settingsDialog(c, "settings-remove-confirm", "Confirm removal", &state.Open, 440, func() {
		ui.Text(c, fmt.Sprintf(tr(c, "Remove “%s”?"), state.Name)).FillWidth()
		ui.Row(c).FillWidth().Justify(ui.End).Gap(8).Children(func() {
			textButton(c.Key("settings-remove-cancel"), tr(c, "Cancel")).OnClick(func() { state.Open = false })
			textButton(c.Key("settings-remove-confirm-action"), tr(c, "Remove")).Label(tr(c, "Confirm delete")).OnClick(func() {
				state.Open = false
				switch state.Kind {
				case "provider":
					a.dispatch(contracts.Command{Kind: "provider.delete", ID: state.ID})
				case "mcp", "skill", "command":
					if state.Kind == "skill" {
						a.capabilityUI.skillDetailsOpen, a.capabilityUI.skillOpen = false, false
					}
					a.dispatch(contracts.Command{Kind: "resource." + state.Kind + ".delete", ID: state.ID})
				case "subagent":
					a.state.Settings.Subagents = slices.DeleteFunc(contracts.CloneSubagentProfiles(a.state.Settings.Subagents),
						func(profile contracts.SubagentProfile) bool { return profile.ID == state.ID })
					if !contracts.IsBuiltinSubagent(state.ID) {
						a.startCustomSubagentSave(state.ID, true)
						return
					}
					a.saveSettings()
				case "hook":
					a.state.Settings.Hooks = slices.DeleteFunc(contracts.CloneHookRules(a.state.Settings.Hooks),
						func(rule contracts.HookRule) bool { return rule.ID == state.ID })
					a.saveSettings()
				}
			})
		})
	})
}

func (a *App) selectSettingsSection(section string) {
	if a.section == "Personalization" && section != a.section && a.agentsDraft != a.agentsSaved {
		a.saveGlobalAgentsDraft()
	}
	if a.section == "Model settings" && section != a.section && a.modelsUI.inlineID != "" {
		if a.modelsUI.inlineSaving {
			a.modelsUI.nextSection = section
			return
		}
		if a.modelsUI.inlineDirty {
			a.modelsUI.nextSection = section
			if !a.saveInlineProvider() {
				a.modelsUI.nextSection = ""
			}
			return
		}
	}
	a.runtimeUI.subagentOpen = false
	a.runtimeUI.key, a.runtimeUI.provider.ID, a.runtimeUI.subagent.ID = "", "", ""
	a.dismissModelFetch()
	a.modelsUI.modelOpen, a.modelsUI.pendingKey, a.modelsUI.pendingCommand = false, "", ""
	a.modelsUI.inlineID, a.modelsUI.inlineError = "", ""
	a.modelsUI.inlineDirty, a.modelsUI.showKey = false, false
	a.modelsUI.revealedKey, a.modelsUI.keyLoading = "", false
	a.modelsUI.keyRequest++
	state := &a.capabilityUI
	state.plugins.configID, state.plugins.config, state.plugins.removeConfig = "", nil, nil
	state.plugins.sourcesOpen, state.plugins.sourceOpen = false, false
	state.plugins.source = contracts.PluginMarketplaceInput{}
	state.mcpOpen, state.skillOpen, state.commandOpen, state.hookOpen = false, false, false, false
	state.skillDetailsOpen, state.skillImportOpen = false, false
	state.mcpEnv, state.mcpHeaders, state.mcpJSON = "", "", ""
	state.pendingHookSave, state.pendingSubagentSave = nil, nil
	a.settingsRemoval.Open = false
	a.settingsImport = settingsImportState{}
	a.section = section
	if section == "Usage statistics" {
		a.refreshAppUsage()
	}
}
