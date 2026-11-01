package desktop

import (
	"crypto/rand"
	"gcode/internal/contracts"
	"gcode/internal/i18n"
)

type branchCreation struct {
	requestID, parentID string
	side                bool
}

func (a *App) canNewSideChat() bool {
	session := a.currentSession()
	return a.route == "chat" && session.ID != "" && !session.Archived && session.SubagentType == "" && a.branchCreation == nil
}

func (a *App) newSideChat() {
	if a.canNewSideChat() {
		a.createRelatedSession(a.sessionID, true)
	}
}

func (a *App) createRelatedSession(parentID string, side bool) {
	if a.branchCreation != nil || !a.hasSession(parentID) {
		return
	}
	request := &branchCreation{requestID: "branch:" + rand.Text(), parentID: parentID, side: side}
	a.branchCreation = request
	a.errorText = ""
	kind, name := "session.sidechat", a.t("Side chat")
	if !side {
		kind = "session.fork"
		for _, session := range a.state.Sessions {
			if session.ID == parentID {
				name = i18n.In(a.state.Settings.General.Language, "Fork of %s", session.Title)
				break
			}
		}
	}
	a.dispatch(contracts.Command{Kind: kind, SessionID: parentID, RequestID: request.requestID, Name: name})
}

func (a *App) openRelatedSession(session contracts.Session) {
	if !a.hasSession(session.ID) || session.Archived {
		return
	}
	if session.SideChat {
		a.sideComposer(session.ID).FocusPending = true
		a.workspace.OpenSession(session.ID)
	} else {
		a.navigate("chat/" + session.ID)
	}
}

func (a *App) ownsBranchCommand(command contracts.Command) bool {
	request := a.branchCreation
	if request == nil || command.RequestID != request.requestID || command.SessionID != request.parentID {
		return false
	}
	return request.side && command.Kind == "session.sidechat" || !request.side && command.Kind == "session.fork"
}

func (a *App) branchCommandFinished(command contracts.Command, err error) bool {
	if !a.ownsBranchCommand(command) {
		return false
	}
	if err != nil {
		a.branchCreation = nil
		a.errorText = err.Error()
	}
	return true
}

func (a *App) reconcileBranchCreation() {
	request := a.branchCreation
	if request == nil {
		return
	}
	for _, session := range a.state.Sessions {
		if session.CreationRequestID != request.requestID || session.ParentSessionID != request.parentID || session.SideChat != request.side {
			continue
		}
		a.branchCreation = nil
		// A late completion may update the data, but cannot take over another chat.
		if a.sessionID == request.parentID && a.route == "chat" {
			a.openRelatedSession(session)
		}
		return
	}
}

func (a *App) branchCommandApplied(command contracts.Command, err error) {
	if err == nil && a.ownsBranchCommand(command) {
		a.branchCreation = nil
		a.errorText = a.t("The new conversation is no longer available.")
	}
}

func (a *App) branchSnapshotFailed(command contracts.Command, err error) {
	if err != nil && a.ownsBranchCommand(command) {
		a.branchCreation = nil
		a.errorText = err.Error()
	}
}
