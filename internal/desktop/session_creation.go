package desktop

import (
	"crypto/rand"
	"gcode/internal/contracts"
	"gcode/internal/i18n"
)

type branchCreation struct {
	requestID, parentID, kind string
	worktree                  bool
}

func (a *App) canNewSideChat() bool {
	session := a.currentSession()
	return a.route == "chat" && session.ID != "" && !session.Archived && session.SubagentType == "" && a.branchCreation == nil
}

func (a *App) newSideChat() {
	if a.canNewSideChat() {
		a.createSideChat(a.sessionID)
	}
}

func (a *App) createSideChat(parentID string) {
	if a.branchCreation != nil || !a.hasSession(parentID) {
		return
	}
	a.branchCreation = &branchCreation{requestID: "branch:" + rand.Text(), parentID: parentID, kind: "session.sidechat"}
	a.errorText = ""
	a.dispatch(contracts.Command{Kind: "session.sidechat", SessionID: parentID, RequestID: a.branchCreation.requestID, Name: a.t("Side chat")})
}

// canBranchSession reports whether the sidebar may offer "Branch chat" for the
// conversation. A branch is the user's own conversation, so a side chat or a
// subagent task is not a source the list offers.
func (a *App) canBranchSession(session contracts.Session) bool {
	return a.branchCreation == nil && session.TopLevel() && !session.Archived && a.hasSession(session.ID)
}

// canBranchSessionWorktree also requires a Git repository: the branch gets its
// own worktree on a new branch from the current HEAD.
func (a *App) canBranchSessionWorktree(session contracts.Session) bool {
	if !a.canBranchSession(session) || session.ProjectID == "" {
		return false
	}
	for _, project := range a.state.Projects {
		if project.ID == session.ProjectID {
			return project.GitDetected
		}
	}
	return false
}

func (a *App) branchSession(session contracts.Session, worktree bool) {
	if worktree && !a.canBranchSessionWorktree(session) || !worktree && !a.canBranchSession(session) {
		return
	}
	request := &branchCreation{requestID: "branch:" + rand.Text(), parentID: session.ID, kind: "session.branch", worktree: worktree}
	a.branchCreation = request
	a.errorText = ""
	command := contracts.Command{
		Kind: "session.branch", SessionID: session.ID, RequestID: request.requestID,
		Name: i18n.In(a.state.Settings.General.Language, "Branch of %s", session.Title),
	}
	if worktree {
		command.Checkout = &contracts.CheckoutPlan{Kind: contracts.CheckoutNew}
	}
	a.dispatch(command)
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
	if request == nil || command.RequestID != request.requestID || command.SessionID != request.parentID || command.Kind != request.kind {
		return false
	}
	return (command.Checkout != nil) == request.worktree
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
		if session.CreationRequestID != request.requestID {
			continue
		}
		// A branch is a conversation of its own; a side chat stays a child of
		// the conversation that created it.
		if request.kind == "session.sidechat" && (!session.SideChat || session.ParentSessionID != request.parentID) {
			continue
		}
		if request.kind == "session.branch" && (session.SideChat || session.ParentSessionID != "") {
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
