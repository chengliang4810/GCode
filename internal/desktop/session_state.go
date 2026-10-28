package desktop

import (
	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
	"strings"
)

type sessionViewState struct {
	Annotations            []contracts.Annotation
	Draft                  string
	Attachments            []contracts.Attachment
	Files, Right, Terminal bool
	RightExpanded          bool
	PanelOpened            [3]uint64
	Messages               ui.ListState
}

func (a *App) newConversation() {
	a.draftEpoch++
	a.navigate("chat")
	a.draft = ""
	a.attachments = nil
	a.annotations = nil
	a.workspace.ResetAnnotations()
	a.drafts[""] = ""
	delete(a.sessionStates, "")
}

// NewConversation opens an empty draft from the native application menu.
func (a *App) NewConversation() { a.newConversation() }

func (a *App) storeSessionView() {
	if a.sessionStates == nil {
		a.sessionStates = make(map[string]sessionViewState)
	}
	a.drafts[a.sessionID] = a.draft
	a.sessionStates[a.sessionID] = sessionViewState{Draft: a.draft, Attachments: append([]contracts.Attachment(nil), a.attachments...), Annotations: append([]contracts.Annotation(nil), a.annotations...), Files: a.files, Right: a.right, Terminal: a.terminal, RightExpanded: a.rightExpanded, PanelOpened: a.panelOpened, Messages: a.messages}
}
func (a *App) restoreSessionView(id string) {
	v := a.sessionStates[id]
	a.draft = v.Draft
	a.attachments = append([]contracts.Attachment(nil), v.Attachments...)
	a.annotations = append([]contracts.Annotation(nil), v.Annotations...)
	indices := make([]uint32, 0, len(a.annotations))
	for _, annotation := range a.annotations {
		indices = append(indices, annotation.Index)
	}
	a.workspace.ReserveAnnotationIndices(indices)
	a.workspace.KeepAnnotations(indices)
	a.files = v.Files
	a.right = v.Right
	a.terminal = v.Terminal
	if id == "" {
		a.terminal = false
	}
	a.rightExpanded = id != "" && v.Right && v.RightExpanded
	a.panelOpened = v.PanelOpened
	if a.panelOpened == [3]uint64{} {
		a.panelOpened = [3]uint64{1, 2, 3}
	}
	a.messages = v.Messages
}
func (a *App) commandFinished(cmd contracts.Command, err error) {
	if cmd.Kind == "message.send" && cmd.Send != nil {
		if pending, ok := a.pendingSends[cmd.Send.SessionID]; ok && pending.RequestID == cmd.Send.RequestID {
			delete(a.pendingSends, cmd.Send.SessionID)
		}
	}
	if a.projectCommandFinished(cmd, err) {
		return
	}
	if a.integrationCommandFinished(cmd, err) {
		return
	}
	if a.sideCommandFinished(cmd, err) {
		return
	}
	if cmd.Kind == "settings.update" {
		if a.settingsPending > 0 {
			a.settingsPending--
		}
		if a.settingsPending == 0 {
			a.desiredSettings = nil
			a.state.Settings = copySettings(a.authoritativeSettings)
			a.syncNativeAppearance()
			a.refreshBackground()
		}
	}
	if err == nil {
		return
	}
	a.errorText = err.Error()
	if cmd.Kind == "message.send" && cmd.Send != nil {
		req := cmd.Send
		text := req.Text
		for i := len(req.Annotations) - 1; i >= 0; i-- {
			text = strings.TrimSuffix(text, "\n\n"+req.Annotations[i].Link())
		}
		if req.SessionID == "" {
			a.pendingNew = false
		}
		if a.sessionID == req.SessionID {
			if a.draft == "" {
				a.draft = text
				a.attachments = append([]contracts.Attachment(nil), req.Attachments...)
				a.annotations = append([]contracts.Annotation(nil), req.Annotations...)
			}
		} else {
			if a.sessionStates == nil {
				a.sessionStates = make(map[string]sessionViewState)
			}
			v := a.sessionStates[req.SessionID]
			if v.Draft == "" {
				v.Draft = text
				v.Attachments = append([]contracts.Attachment(nil), req.Attachments...)
				v.Annotations = append([]contracts.Annotation(nil), req.Annotations...)
				a.sessionStates[req.SessionID] = v
				a.drafts[req.SessionID] = text
			}
		}
	}
}

func (a *App) selectedModel() contracts.Model {
	for _, m := range a.state.Models {
		if m.ID == a.modelID {
			return m
		}
	}
	return contracts.Model{}
}
func (a *App) modelChanged() {
	a.normalizeReasoning()
	a.normalizeRunConfig()
	if model := a.selectedModel(); model.ID != "" {
		if a.state.Settings.ModelPicker.Selections == nil {
			a.state.Settings.ModelPicker.Selections = make(map[string]contracts.ModelSelection)
		}
		a.state.Settings.ModelPicker.Selections[model.ProviderID] = contracts.ModelSelection{ModelID: model.ID, Reasoning: a.reasoning}
		a.state.Settings.ModelPicker.ProviderID = model.ProviderID
		a.saveSettings()
	}
	if a.sessionID != "" {
		model, reasoning := a.modelID, a.reasoning
		mode, plan := a.accessMode, a.plan
		a.dispatch(contracts.Command{Kind: "session.update", Session: &contracts.SessionUpdate{ID: a.sessionID, ModelID: &model, ReasoningLevel: &reasoning, AccessMode: &mode, Plan: &plan}})
	}
}

func (a *App) normalizeReasoning() {
	m := a.selectedModel()
	if len(m.ReasoningLevels) == 0 {
		a.reasoning = ""
		return
	}
	valid := false
	for _, level := range m.ReasoningLevels {
		if level == a.reasoning {
			valid = true
		}
	}
	if !valid && len(m.ReasoningLevels) > 0 {
		a.reasoning = m.ReasoningLevels[0]
		for _, level := range m.ReasoningLevels {
			if level == "High" {
				a.reasoning = level
			}
		}
	}
}
