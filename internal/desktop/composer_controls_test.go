package desktop

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func TestRunningComposersAllowSilentProviderAndReasoningChanges(t *testing.T) {
	for _, side := range []bool{false, true} {
		for _, status := range []contracts.SessionStatus{contracts.Working, contracts.AwaitingInput} {
			for _, appearance := range []string{"light", "dark"} {
				t.Run(fmt.Sprintf("side=%t/%s/%s", side, status, appearance), func(t *testing.T) {
					var a *App
					var child contracts.Session
					if side {
						a, _, child = sideViewTester(t)
					} else {
						a = testApp(t, "default")
						a.selectSession("chat-0")
					}
					sid, suffix := a.sessionID, ""
					width := 900
					if side {
						sid, suffix, width = child.ID, " in "+child.Title, 1600
					}
					for i := range a.state.Sessions {
						if a.state.Sessions[i].ID == sid {
							a.state.Sessions[i].Status = status
						}
					}
					a.state.Settings.Appearance.Mode = appearance
					recorder := &sideRecordingService{Service: a.service}
					a.service = recorder
					tt := ui.NewTester(a.View, width, 880)
					if err := openTestModelList(tt, "Select model"+suffix); err != nil {
						t.Fatal(err)
					}
					if err := tt.Click("Codex"); err != nil {
						t.Fatal("running composer locked provider:", err)
					}
					if err := tt.Click("Select model" + suffix + " GPT-5.4"); err != nil {
						t.Fatal(err)
					}
					if err := tt.Click("Select reasoning effort" + suffix); err != nil {
						t.Fatal(err)
					}
					if err := tt.Click("Reasoning effort" + suffix); err != nil {
						t.Fatal(err)
					}
					controls := a.centralControls()
					if side {
						controls = a.sideControls(sid, a.sideComposer(sid))
					}
					if controls.reasoningChoice().selected == 0 {
						tt.Key(0, ui.KeyEnd)
					} else {
						tt.Key(0, ui.KeyHome)
					}
					updated := 0
					for _, command := range recorder.commands {
						if command.Kind == "session.update" && command.Session != nil && command.Session.ID == sid {
							updated++
						}
						if command.Kind == "message.stop" {
							t.Fatal("changing settings stopped the run")
						}
					}
					if updated < 2 || a.errorText != "" || (side && a.sideComposer(sid).Failure != "") {
						t.Fatalf("changes were not silently accepted: updates=%d error=%s", updated, a.errorText)
					}
				})
			}
		}
	}
}

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

func TestSharedComposerRunningSideChangesItsProviderIndependently(t *testing.T) {
	a, tt, child := sideViewTester(t)
	for i := range a.state.Sessions {
		if a.state.Sessions[i].ID == child.ID {
			a.state.Sessions[i].Status = contracts.Working
		}
	}
	tt.Frame()
	state := a.sideComposer(child.ID)
	side := a.sideControls(child.ID, state)
	mainModel := a.modelID
	side.activateModelRow([]modelRow{{model: contracts.Model{ID: "gpt-5.4", ProviderID: "openai"}, provider: contracts.Provider{ID: "openai"}}}, 0)
	if state.modelID != "gpt-5.4" || a.modelID != mainModel || state.Failure != "" {
		t.Fatal("running side failed to change provider independently")
	}
}
