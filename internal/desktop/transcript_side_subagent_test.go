package desktop

import (
	"testing"
	"time"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func TestSubagentSideTranscriptIsReadOnlyAndPreservesMainDraft(t *testing.T) {
	app := testApp(t, "transcript")
	app.selectSession("chat-0")
	app.draft = "Keep the central draft"
	var child contracts.Session
	for i := range app.state.Sessions {
		if app.state.Sessions[i].ID == "showcase-agent" {
			app.state.Sessions[i].SubagentType = "Explore"
			app.state.Sessions[i].ModelID = "user/model"
			app.state.Sessions[i].ReasoningLevel = "Max"
			child = app.state.Sessions[i]
		}
	}
	if child.ID == "" {
		t.Fatal("missing child fixture")
	}
	app.state.Messages["chat-0"] = []contracts.Message{{ID: "parent", SessionID: "chat-0", Role: "assistant", Blocks: []contracts.Block{
		{ID: "child", Kind: "subagent", Subagent: &contracts.Subagent{ID: "child", SessionID: child.ID, Name: "Schema reviewer", Status: "completed"}},
	}}}
	stabilizeFixtureSnapshot(t, app, child.ID)
	app.workspace.Update(app.state)
	tt := ui.NewTester(app.View, 1320, 880)
	if err := tt.Click("Open agent: Schema reviewer"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300 * time.Millisecond)
	for _, label := range []string{"Message to " + child.Title, "Send message to " + child.Title, "Attach files to " + child.Title} {
		if _, ok := tt.Find(label); ok {
			t.Fatalf("read-only child exposed %q", label)
		}
	}
	if !tt.HasText("The fixture uses schemaVersion 1") || !tt.HasText("user/model · Max") || app.sessionID != "chat-0" || app.draft != "Keep the central draft" {
		t.Fatal("child transcript did not preserve the actual transcript and main draft")
	}
	state := app.sideComposer(child.ID)
	state.Draft = "Do not send this"
	if app.sendSideMessage(child, state, state.Draft) || state.Pending != nil || state.Draft != "Do not send this" {
		t.Fatal("direct side send bypassed read-only child protection")
	}
	app.dismissSideComposer(child.ID)
	tt.Frame()
	app.workspace.OpenSession(child.ID)
	tt.Frame()
	if _, ok := tt.Find("Message to " + child.Title); ok {
		t.Fatal("reopening a child restored its composer")
	}
}
