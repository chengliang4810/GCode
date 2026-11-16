package desktop

import (
	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func (a *App) loadGlobalAgentsDraft() {
	service, ok := a.service.(contracts.GlobalInstructionsService)
	if !ok {
		a.notification(a.t("Could not save AGENTS.md."))
		return
	}
	if a.agentsLoading || a.agentsSaving || a.agentsDraft != a.agentsSaved {
		return
	}
	a.agentsLoading = true
	a.backgroundTask(func() func() {
		text, err := service.ReadGlobalInstructions(a.ctx)
		return func() {
			a.agentsLoading = false
			if err != nil {
				a.notification(err.Error())
				return
			}
			a.agentsDraft, a.agentsSaved = text, text
		}
	})
}

func (a *App) saveGlobalAgentsDraft() {
	if a.agentsLoading || a.agentsDraft == a.agentsSaved {
		return
	}
	if len(a.agentsDraft) > contracts.MaxGlobalInstructionsBytes {
		a.notification(a.t("AGENTS.md is too large. The limit is 256 KB."))
		return
	}
	if a.agentsSaving {
		a.agentsSavePending = true
		return
	}
	service, ok := a.service.(contracts.GlobalInstructionsService)
	if !ok {
		a.notification(a.t("Could not save AGENTS.md."))
		return
	}
	text := a.agentsDraft
	a.agentsSaving = true
	done := make(chan struct{})
	a.agentsSaveDone = done
	a.backgroundTask(func() func() {
		defer close(done)
		err := service.WriteGlobalInstructions(a.ctx, text)
		return func() {
			a.agentsSaving = false
			pending := a.agentsSavePending
			a.agentsSavePending = false
			if err != nil {
				a.notification(err.Error())
				return
			}
			a.agentsSaved = text
			if pending {
				a.saveGlobalAgentsDraft()
			} else {
				a.notification(a.t("Saved"))
			}
		}
	})
}

func (a *App) personalizationSettings(c *ui.Context) {
	p := &a.state.Settings.Personalization
	sectionTitle(c, tr(c, "Memory"))
	a.settingsCard(c, func() {
		a.settingsRow(c, "Auto memory", "Remember facts about you and your projects across sessions. Off until you turn it on.", func() {
			a.settingsToggle(c, "Auto memory", &p.MemoryEnabled)
		})
	})
	ui.Row(c.Key("agents-actions")).FillWidth().AlignItems(ui.Center).Children(func() {
		sectionTitle(c, tr(c, "Global instructions"))
	})
	input := controlTextArea(c.Key("agents-draft"), &a.agentsDraft).FillWidth().Height(360).Label("AGENTS.md").Disabled(a.agentsLoading)
	settingsInputCommit(input, &a.agentsDraft, a.saveGlobalAgentsDraft)
}
