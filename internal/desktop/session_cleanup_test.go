package desktop

import (
	"slices"
	"testing"

	"gcode/internal/contracts"
)

func TestDeletedSessionReclaimsUIStateWithoutClearingOtherDrafts(t *testing.T) {
	for _, selected := range []bool{false, true} {
		t.Run(map[bool]string{false: "unselected", true: "selected"}[selected], func(t *testing.T) {
			a := testApp(t, "default")
			removed := "chat-0"
			kept := a.state.Sessions[1].ID
			if selected {
				a.selectSession(removed)
			} else {
				a.selectSession(kept)
			}
			a.goalEditor(removed).draft = "remove private draft"
			a.goalEditor(kept).draft = "keep private draft"
			a.drafts[""] = "canvas draft"
			a.drafts[removed], a.drafts[kept] = "removed", "kept"
			a.sideComposer(removed).Draft = "removed side draft"
			a.sideComposer(kept).Draft = "kept side draft"
			a.questionForms = map[string]*questionFormState{"main:" + removed: {}, "side:" + kept: {}}
			a.todoPanels = map[string]*todoPanelState{"side:" + removed: {}, "main:" + kept: {}}
			a.expanded["group:pinned"] = true
			snapshot := contracts.CloneSnapshot(a.state)
			snapshot.Revision++
			snapshot.Sessions = slices.DeleteFunc(snapshot.Sessions, func(s contracts.Session) bool { return s.ID == removed })
			a.apply(snapshot)
			a.pruneSessionUIState() // Reconciliation must be idempotent.
			if a.goalEditors[removed] != nil || a.sideComposers[removed] != nil ||
				a.questionForms["main:"+removed] != nil || a.todoPanels["side:"+removed] != nil {
				t.Fatal("deleted session retained composer or editor state")
			}
			if _, ok := a.drafts[removed]; ok {
				t.Fatal("deleted session retained a draft")
			}
			if a.goalEditors[kept].draft != "keep private draft" || a.sideComposers[kept].Draft != "kept side draft" ||
				a.drafts[kept] != "kept" || a.drafts[""] != "canvas draft" ||
				a.questionForms["side:"+kept] == nil || a.todoPanels["main:"+kept] == nil ||
				!a.expanded["group:pinned"] {
				t.Fatal("cleanup discarded another session or global UI state")
			}
		})
	}
}

func TestSessionUICleanupUsesOnlyAcceptedSessionIdentities(t *testing.T) {
	a := testApp(t, "default")
	a.goalEditor("chat-0").draft = "keep"
	older := contracts.CloneSnapshot(a.state)
	older.Revision--
	older.Sessions = nil
	a.apply(older)
	if a.goalEditors["chat-0"] == nil {
		t.Fatal("ignored stale snapshot removed UI state")
	}
	archived := contracts.CloneSnapshot(a.state)
	archived.Revision++
	for i := range archived.Sessions {
		if archived.Sessions[i].ID == "chat-0" {
			archived.Sessions[i].Archived = true
		}
	}
	archived.Messages = nil // Unloading a body is not deleting the session.
	a.apply(archived)
	if a.goalEditors["chat-0"] == nil || a.goalEditors["chat-0"].draft != "keep" {
		t.Fatal("archiving or unloading discarded the draft")
	}
}
