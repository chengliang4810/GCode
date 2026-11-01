package desktop

import (
	"cmp"
	"slices"
	"strings"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

// An empty ProjectID is an explicit project opt-out, distinct from no bookmark.
type canvasTarget struct {
	ProjectID, DeviceID, ProjectDeviceID string
}

func (a *App) rememberCanvasTarget() {
	a.canvasTarget = &canvasTarget{ProjectID: a.projectID, DeviceID: a.deviceID}
	for _, project := range a.state.Projects {
		if project.ID == a.projectID {
			a.canvasTarget.ProjectDeviceID = project.DeviceID
			return
		}
	}
}

func (a *App) invalidateCanvasCloudTarget(session contracts.Session) {
	target := a.canvasTarget
	if target == nil || !footerCloudDevice(target.DeviceID) {
		return
	}
	if !footerCloudDevice(target.ProjectDeviceID) && (session.ProjectID != target.ProjectID || session.DeviceID != target.ProjectDeviceID) {
		// The source forgets its synthetic Cloud checkout on a project
		// or effective-host change; it cannot silently return afterward.
		target.DeviceID = target.ProjectDeviceID
	}
}

func (a *App) restoreCanvasTarget() {
	if a.canvasTarget == nil {
		return
	}
	target := *a.canvasTarget
	a.canvasTarget = nil
	pickedDevice := target.DeviceID
	syntheticCloud := footerCloudDevice(target.DeviceID) && target.ProjectDeviceID != "" && !footerCloudDevice(target.ProjectDeviceID)
	if syntheticCloud {
		pickedDevice = target.ProjectDeviceID
	}
	a.projectID, a.deviceID = target.ProjectID, pickedDevice
	if target.ProjectID == "" {
		return
	}
	projects := slices.Clone(a.state.Projects)
	slices.SortFunc(projects, func(x, y contracts.Project) int {
		if order := cmp.Compare(strings.ToLower(x.Name), strings.ToLower(y.Name)); order != 0 {
			return order
		}
		return cmp.Compare(x.ID, y.ID)
	})
	var picked *contracts.Project
	for i := range projects {
		if projects[i].ID == target.ProjectID {
			picked = &projects[i]
			break
		}
	}
	if picked == nil {
		for i := range projects {
			if projects[i].DeviceID == pickedDevice {
				picked = &projects[i]
				break
			}
		}
		if picked == nil && len(projects) > 0 {
			picked = &projects[0]
		}
	}
	a.projectID, a.branch = "", ""
	if picked != nil {
		a.projectID, a.branch = picked.ID, picked.Branch
		// Effective device follows the chosen project's host. Only the same
		// project/host may retain its synthetic Cloud checkout display.
		a.deviceID = picked.DeviceID
		if syntheticCloud && picked.ID == target.ProjectID && picked.DeviceID == target.ProjectDeviceID {
			a.deviceID = target.DeviceID
		}
	}
}

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
	a.pendingNew = false
	a.pendingNewRequestID = ""
	delete(a.pendingSends, "")
	a.navigate("chat")
	a.terminal = false
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
	if scope := a.sidebarUI.pullRequests; scope != nil && cmd.Kind == "git.refs.load" &&
		cmd.Git != nil && cmd.Git.Scope == *scope && err != nil {
		a.sidebarUI.pullRequests = nil
		a.errorText = err.Error()
	}
	a.capabilityCommandFinished(cmd, err)
	a.modelSettingsCommandFinished(cmd, err)
	a.settingsImportCommandFinished(cmd, err)
	if a.branchCommandFinished(cmd, err) {
		return
	}
	primaryCurrent := false
	if cmd.Kind == "message.send" && cmd.Send != nil {
		if pending, ok := a.pendingSends[cmd.Send.SessionID]; ok && pending.RequestID == cmd.Send.RequestID {
			primaryCurrent = cmd.Send.SessionID != "" || pending.DraftEpoch == a.draftEpoch
			delete(a.pendingSends, cmd.Send.SessionID)
		}
	}
	if a.projectCommandFinished(cmd, err) {
		return
	}
	if err != nil && cmd.Kind == "session.update" && cmd.Session != nil && a.sidebarUI.revealID == cmd.Session.ID {
		a.sidebarUI.revealID, a.sidebarUI.revealGroup = "", ""
	}
	if a.composerFooterGitCommandFinished(cmd, err) || a.footerUsageCommandFinished(cmd, err) {
		return
	}
	if a.integrationCommandFinished(cmd, err) {
		return
	}
	if a.sideCommandFinished(cmd, err) {
		return
	}
	if cmd.Kind == "message.send" && cmd.Send != nil && (!primaryCurrent || cmd.Send.SessionID != "" && !a.hasSession(cmd.Send.SessionID)) {
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
			a.syncNativeLanguage()
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
			a.pendingNewRequestID = ""
			// The session was never minted: the composer returns to the
			// canvas along the same dock clock it left on.
			a.launchingNewSession = false
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
		a.dispatch(contracts.Command{Kind: "session.update", Session: &contracts.SessionUpdate{ID: a.sessionID, ModelID: &model, ReasoningLevel: &reasoning}})
	}
}

func (a *App) normalizeReasoning() {
	a.centralControls().normalizeReasoning()
}
