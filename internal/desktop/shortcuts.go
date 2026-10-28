package desktop

import "gcode/internal/contracts"

func (a *App) stopRun() {
	if a.paletteOpen || a.modelMenu || a.projectMenu || a.sidebarProjectMenu || a.deviceMenu || a.viewMenu || a.branchMenu || a.imageOpen || a.renameOpen || a.projectOpen || a.attachmentOpen || a.actionOpen || a.background.Open || a.errorText != "" || a.integrationHasSurface() {
		return
	}
	status := a.currentSession().Status
	if status == contracts.Working || status == contracts.AwaitingInput {
		a.dispatch(contracts.Command{Kind: "message.stop", SessionID: a.sessionID})
	}
}
