package desktop

import (
	"context"
	"errors"
	"testing"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func TestSharedComposerSideModelKeepsConfigurationIndependent(t *testing.T) {
	a, tt, child := sideViewTester(t)
	for _, label := range []string{"Access mode", "Access mode in " + child.Title} {
		if _, visible := tt.Find(label); visible {
			t.Fatalf("removed access control is still visible: %s", label)
		}
	}
	mainModel, mainReason := a.modelID, a.reasoning
	recorder := &sideRecordingService{Service: a.service}
	a.service = recorder
	if err := openTestModelList(tt, "Select model in "+child.Title); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Codex"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Search models in " + child.Title); err != nil {
		t.Fatal(err)
	}
	tt.Type("GPT-5.3")
	tt.Key(0, ui.KeyEnter)
	state := a.sideComposer(child.ID)
	if state.modelID != "gpt-5.3-codex" || state.modelMenu {
		t.Fatalf("side model picker state %+v", state.composerControls)
	}
	if a.modelID != mainModel || a.reasoning != mainReason || a.modelMenu {
		t.Fatal("side model choice mutated central controls")
	}
	tt.Key(0, ui.KeyEscape)
	snapshot, err := a.service.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, session := range snapshot.Sessions {
		if session.ID == child.ID {
			if session.ModelID != state.modelID || session.ReasoningLevel != state.reasoning {
				t.Fatalf("side backend config differs: %+v", session)
			}
		}
	}
	if err := tt.Click("Message to " + child.Title); err != nil {
		t.Fatal(err)
	}
	tt.Type("Plan a schema review")
	tt.Key(0, ui.KeyEnter)
	var sent *contracts.SendRequest
	for _, command := range recorder.commands {
		if command.Kind == "message.send" {
			sent = command.Send
		}
	}
	if sent == nil || sent.SessionID != child.ID || sent.ModelID != state.modelID ||
		sent.ReasoningLevel != state.reasoning {
		t.Fatalf("side send lost selected config: %+v", sent)
	}
}

func TestSharedComposerSideConfigFailureRestoresItsSnapshot(t *testing.T) {
	a, tt, child := sideViewTester(t)
	recorder := &sideRecordingService{Service: a.service, updateErr: errors.New("Unable to update this conversation")}
	a.service = recorder
	if err := openTestModelList(tt, "Select model in "+child.Title); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Codex"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Search models in " + child.Title); err != nil {
		t.Fatal(err)
	}
	tt.Type("GPT-5.3")
	tt.Key(0, ui.KeyEnter)
	state := a.sideComposer(child.ID)
	if len(state.configPending) != 0 || state.modelID != child.ModelID ||
		state.reasoning != child.ReasoningLevel || state.Failure == "" || a.errorText != "" {
		t.Fatalf("failed side update leaked or failed to restore %+v, %q", state.composerControls, a.errorText)
	}
}

func TestSharedComposerRunningSideLocksItsProviderOnly(t *testing.T) {
	a, tt, child := sideViewTester(t)
	for i := range a.state.Sessions {
		if a.state.Sessions[i].ID == child.ID {
			a.state.Sessions[i].Status = contracts.Working
		}
	}
	tt.Frame()
	state := a.sideComposer(child.ID)
	side := a.sideControls(child.ID, state)
	if !side.modelProviderLocked() || a.modelProviderLocked() {
		t.Fatal("provider lock used the other composer's session")
	}
	current := side.selectedModel().ProviderID
	for _, row := range side.modelRows() {
		if row.provider.ID != current {
			t.Fatal("running side offered another provider")
		}
	}
	old := state.modelID
	side.activateModelRow([]modelRow{{model: contracts.Model{ID: "gpt-5.4", ProviderID: "openai"}, provider: contracts.Provider{ID: "openai"}}}, 0)
	if state.modelID != old {
		t.Fatal("running side changed provider")
	}
}
