package desktop

import (
	"testing"
	"time"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func TestFastModeUsesSourceOptionEncodingsAndKeepsOtherOptions(t *testing.T) {
	for _, tc := range []struct {
		name, id, initial, on, off string
		choices                    []string
		fast                       bool
	}{
		{"Codex tier", "serviceTier", "default", "fast", "default", []string{"default", "fast"}, true},
		{"Claude toggle", "fastMode", "off", "on", "off", []string{"off", "on"}, true},
		{"snake case", "fast_mode", "off", "on", "off", []string{"off", "on"}, true},
		{"speed", "speed", "standard", "fast", "standard", []string{"standard", "fast"}, true},
		{"Cursor default on", "fast", "true", "true", "false", []string{"false", "true"}, true},
		{"generic toggle stays a row", "thinking", "off", "", "", []string{"off", "on"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := testApp(t, "default")
			modelID := a.modelID
			for i := range a.state.Models {
				if a.state.Models[i].ID == modelID {
					a.state.Models[i].Options = []contracts.ModelOption{
						{ID: tc.id, Label: "Candidate preference", Choices: tc.choices, DefaultChoice: tc.initial},
						{ID: "contextWindow", Label: "Context window", Choices: []string{"200k", "1m"}, DefaultChoice: "200k"},
					}
				}
			}
			a = modelSnapshotApp(t, a.state)
			tt := ui.NewTester(a.View, 1320, 880)
			if err := tt.Click("Select model"); err != nil {
				t.Fatal(err)
			}
			_, fastButton := tt.Find("Fast mode")
			if fastButton != tc.fast || tt.HasText("Candidate preference") == tc.fast || !tt.HasText("Context window") {
				t.Fatalf("model option controls did not follow the source encoding: %v", tt.Texts())
			}
			key := modelID + "/" + tc.id
			tt.Key(0, ui.KeyF)
			if !tc.fast {
				if a.state.Settings.ModelPicker.Options[key] != "" {
					t.Fatal("F changed a generic on/off option")
				}
				return
			}
			want := tc.on
			if tc.initial == tc.on {
				want = tc.off
			}
			if a.state.Settings.ModelPicker.Options[key] != want {
				t.Fatalf("F selected %q, want %q", a.state.Settings.ModelPicker.Options[key], want)
			}
			tt.Key(ui.Shift, ui.KeyF)
			if a.state.Settings.ModelPicker.Options[key] != want {
				t.Fatal("modified F toggled fast mode")
			}
			if err := tt.Click("Fast mode"); err != nil {
				t.Fatal(err)
			}
			if _, ok := a.state.Settings.ModelPicker.Options[key]; ok {
				t.Fatal("returning fast mode to its default left a redundant explicit value")
			}
		})
	}
}

func TestOptionEffortUsesIndependentSliderAndKeepsOtherOptionsInteractive(t *testing.T) {
	for _, optionID := range []string{"effort", "reasoning", "reasoning_effort"} {
		t.Run(optionID, func(t *testing.T) {
			a := testApp(t, "default")
			modelID := a.modelID
			for i := range a.state.Models {
				if a.state.Models[i].ID == modelID {
					a.state.Models[i].ReasoningLevels = nil
					a.state.Models[i].Options = []contracts.ModelOption{
						{ID: optionID, Label: "Option effort", Choices: []string{"low", "medium", "high"}, DefaultChoice: "medium"},
						{ID: "contextWindow", Label: "Context window", Choices: []string{"200k", "1m"}, DefaultChoice: "200k"},
					}
				}
			}
			a = modelSnapshotApp(t, a.state)
			tt := ui.NewTester(a.View, 1320, 880)
			if err := tt.Click("Select reasoning effort"); err != nil {
				t.Fatal(err)
			}
			if _, ok := tt.Find("Reasoning effort"); !ok || tt.HasText("Option effort") {
				t.Fatal("option-backed effort was duplicated instead of using the independent slider")
			}
			tt.Key(0, ui.KeyEnd)
			key := modelID + "/" + optionID
			if a.state.Settings.ModelPicker.Options[key] != "high" || a.effortValue != 2 || a.reasoning != "" {
				t.Fatalf("option effort key changed the wrong configuration: %+v / %s", a.state.Settings.ModelPicker.Options, a.reasoning)
			}
			tt.Key(0, ui.KeyLeft)
			if _, ok := a.state.Settings.ModelPicker.Options[key]; ok || a.effortValue != 1 {
				t.Fatal("returning to default effort did not clear its explicit option")
			}
			tt.Key(0, ui.KeyEscape)
			if err := tt.Click("Select model"); err != nil {
				t.Fatal(err)
			}
			if err := clickAccessible(tt, ui.RoleMenuButton, "Context window"); err != nil {
				t.Fatal(err)
			}
			if err := tt.ChooseMenuItem("1m"); err != nil {
				t.Fatal(err)
			}
			if a.state.Settings.ModelPicker.Options[modelID+"/contextWindow"] != "1m" || !a.modelMenu {
				t.Fatal("another model option stopped responding after keyboard effort changes")
			}
		})
	}
}

func TestModelOptionChoiceLabelsDisplayAndPersistSeparateIDs(t *testing.T) {
	for _, labels := range []bool{true, false} {
		name := "labels"
		if !labels {
			name = "choice ids"
		}
		t.Run(name, func(t *testing.T) {
			a := testApp(t, "default")
			modelID := a.modelID
			option := contracts.ModelOption{ID: "contextWindow", Label: "Context window", Choices: []string{"standard", "1m"}, DefaultChoice: "standard"}
			next := "1m"
			if labels {
				option.ChoiceLabels = map[string]string{"standard": "Standard", "1m": "1M tokens"}
				next = "1M tokens"
			}
			for i := range a.state.Models {
				if a.state.Models[i].ID == modelID {
					a.state.Models[i].Options = []contracts.ModelOption{option}
				}
			}
			a = modelSnapshotApp(t, a.state)
			recorder := &sideRecordingService{Service: a.service}
			a.service = recorder
			tt := ui.NewTester(a.View, 1320, 880)
			if err := tt.Click("Select model"); err != nil {
				t.Fatal(err)
			}
			if err := clickAccessible(tt, ui.RoleMenuButton, "Context window"); err != nil {
				t.Fatal(err)
			}
			if err := tt.ChooseMenuItem(next); err != nil {
				t.Fatal(err)
			}
			key := modelID + "/contextWindow"
			if !tt.HasText(next) || a.state.Settings.ModelPicker.Options[key] != "1m" {
				t.Fatalf("option label replaced its business id: %+v", a.state.Settings.ModelPicker.Options)
			}
			for _, command := range recorder.commands {
				if command.Kind == "settings.update" && command.Settings != nil && command.Settings.ModelPicker.Options[key] == "1m" {
					return
				}
			}
			t.Fatal("option id did not reach the typed settings command")
		})
	}
}

func TestOptionEffortUsesLabelsWithoutSavingLabels(t *testing.T) {
	a := testApp(t, "default")
	modelID := a.modelID
	for i := range a.state.Models {
		if a.state.Models[i].ID == modelID {
			a.state.Models[i].ReasoningLevels = nil
			a.state.Models[i].Options = []contracts.ModelOption{{
				ID: "reasoning_effort", Label: "Option effort", Choices: []string{"low", "medium", "high"}, DefaultChoice: "medium",
				ChoiceLabels: map[string]string{"low": "Quiet", "medium": "Balanced", "high": "Deep"},
			}}
		}
	}
	a = modelSnapshotApp(t, a.state)
	tt := ui.NewTester(a.View, 1320, 880)
	if err := tt.Click("Select reasoning effort"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Balanced") {
		t.Fatal("option effort title did not use its label")
	}
	tt.Key(0, ui.KeyEnd)
	if !tt.HasText("Deep") || a.state.Settings.ModelPicker.Options[modelID+"/reasoning_effort"] != "high" || a.effortValue != 2 {
		t.Fatal("option effort label and id lost their separation")
	}
}

func TestSideReasoningPickerPreservesOwnership(t *testing.T) {
	a, tt, child := sideViewTester(t)
	mainModel, mainReason := a.modelID, a.reasoning
	recorder := &sideRecordingService{Service: a.service}
	a.service = recorder
	if err := tt.Click("Select reasoning effort in " + child.Title); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyHome)
	state := a.sideComposer(child.ID)
	if state.reasoning != "Low" || a.reasoning != mainReason || a.modelID != mainModel || a.effortMenu {
		t.Fatal("side reasoning changed the main composer")
	}
	var updated bool
	for _, command := range recorder.commands {
		if command.Kind == "session.update" && command.Session != nil {
			if command.Session.ID != child.ID {
				t.Fatal("wrong session updated")
			}
			updated = true
		}
	}
	if !updated {
		t.Fatal("side effort bypassed typed session update")
	}
	tt.Key(0, ui.KeyEscape)
	if state.effortMenu {
		t.Fatal("side effort popup did not close")
	}
}

func TestReasoningPickerKeyboardAndModelSearchRemainIndependent(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 1320, 880)
	if err := tt.Click("Select reasoning effort"); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		key  ui.Key
		want string
	}{{ui.KeyEnd, "Max"}, {ui.KeyLeft, "High"}, {ui.KeyHome, "Low"}, {ui.KeyRight, "Medium"}} {
		tt.Key(0, step.key)
		if a.reasoning != step.want || a.modelMenu || !a.effortMenu {
			t.Fatalf("effort key selected %q want %q", a.reasoning, step.want)
		}
	}
	selected, reason := a.modelID, a.reasoning
	if err := openTestModelList(tt, "Select model"); err != nil {
		t.Fatal(err)
	}
	if !tt.Focused("Search models") || a.effortMenu {
		t.Fatal("model list did not focus search independently")
	}
	tt.Key(0, ui.KeyDown)
	if a.modelID != selected || a.reasoning != reason {
		t.Fatal("highlighting a model changed committed config")
	}
	tt.Key(0, ui.KeyEscape)
	if a.modelMenu {
		t.Fatal("Escape did not dismiss model list")
	}
	if err := openTestModelList(tt, "Select model"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(600 * time.Millisecond)
	if !tt.Focused("Search models") {
		t.Fatal("remounted native search did not keep focus")
	}
}
