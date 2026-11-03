package desktop

import (
	"os"
	"path/filepath"

	"github.com/egoist/mygo/ui"
)

// globalAgentsPath resolves ~/.gcode/AGENTS.md from the engine-reported config
// directory, falling back to the home anchor while the runtime is not
// connected. The engine loads this same file through loadInstructions.
func (a *App) globalAgentsPath() string {
	if a.state.Runtime != nil && a.state.Runtime.ConfigDirectory != "" {
		return filepath.Join(a.state.Runtime.ConfigDirectory, "AGENTS.md")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".gcode", "AGENTS.md")
	}
	return ""
}

// loadGlobalAgentsDraft reads the global instructions file for editing. The
// byte cap matches loadInstructions in the engine, which skips longer files.
func (a *App) loadGlobalAgentsDraft() {
	a.agentsDraft = ""
	defer func() { a.agentsSaved = a.agentsDraft }()
	path := a.globalAgentsPath()
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	if len(data) > 256<<10 {
		data = data[:256<<10]
	}
	a.agentsDraft = string(data)
}

func (a *App) saveGlobalAgentsDraft() {
	if len(a.agentsDraft) > 256<<10 {
		a.notification(a.t("AGENTS.md is too large. The limit is 256 KB."))
		return
	}
	path := a.globalAgentsPath()
	if path == "" {
		a.notification(a.t("Could not save AGENTS.md."))
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		a.notification(a.t("Could not save AGENTS.md."))
		return
	}
	if err := os.WriteFile(path, []byte(a.agentsDraft), 0o600); err != nil {
		a.notification(a.t("Could not save AGENTS.md."))
		return
	}
	a.agentsSaved = a.agentsDraft
	a.notification(a.t("Saved"))
}

func (a *App) personalizationSettings(c *ui.Context) {
	p := &a.state.Settings.Personalization
	sectionTitle(c, tr(c, "Memory"))
	a.settingsCard(c, func() {
		a.settingsRow(c, "Auto memory", "Remember facts about you and your projects across sessions. Off until you turn it on.", func() {
			a.settingsToggle(c, "Auto memory", &p.MemoryEnabled)
		})
	})
	if a.agentsLoadedFor != "Personalization" {
		a.loadGlobalAgentsDraft()
		a.agentsLoadedFor = "Personalization"
	}
	ui.Row(c.Key("agents-actions")).FillWidth().AlignItems(ui.Center).Children(func() {
		sectionTitle(c, tr(c, "Global instructions"))
	})
	input := controlTextArea(c.Key("agents-draft"), &a.agentsDraft).FillWidth().Height(360).Label("AGENTS.md")
	settingsInputCommit(input, &a.agentsDraft, a.saveGlobalAgentsDraft)
}
