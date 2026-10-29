package desktop

import (
	"testing"
	"time"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func TestCompactModelListKeyboardHighlightShowsFavoriteControlAndFocusesSearch(t *testing.T) {
	a := modelSurfaceTestApp(t, "dark", true)
	a.state.Settings.ModelPicker.Favorites = nil
	a = modelSnapshotApp(t, a.state)
	tt := ui.NewTester(a.View, 1320, 880)
	if err := tt.Click("Select model"); err != nil {
		t.Fatal(err)
	}
	tt.Move(1200, 100)
	tt.Key(0, ui.KeyEnter)
	if !tt.Focused("Search models") {
		t.Fatal("Enter did not transfer keyboard focus from the panel to the search input")
	}
	if caret, active := tt.TextCaret(); !active || caret.H <= 0 {
		t.Fatalf("search did not publish its native input caret: %+v, active=%v", caret, active)
	}
	selected := a.modelID
	rows := a.modelRows()
	initial := a.modelCursor
	starPainted := func(index int) bool {
		t.Helper()
		box, ok := tt.Find("Add " + rows[index].model.Label + " to favorites")
		if !ok {
			t.Fatalf("favorite control missing for %s", rows[index].model.Label)
		}
		// The source keeps a 24px slot and paints the 13px star only on
		// the active or starred row. Its ink is brighter than the row fill.
		for y := int(box.Y + 5); y < int(box.Y+box.H-5); y++ {
			for x := int(box.X + 5); x < int(box.X+box.W-5); x++ {
				pixel := tt.Image().RGBAAt(x, y)
				if pixel.R > 120 && pixel.G > 120 && pixel.B > 120 {
					return true
				}
			}
		}
		return false
	}
	if !starPainted(initial) {
		t.Fatal("the keyboard's initial model row did not expose its outline favorite star")
	}
	tt.Key(0, ui.KeyDown)
	next := a.modelCursor
	if next == initial || !starPainted(next) || starPainted(initial) || a.modelID != selected {
		t.Fatalf("the outline star did not follow keyboard highlight independently of the committed model: initial=%d next=%d initialPaint=%v nextPaint=%v model=%s want=%s", initial, next, starPainted(initial), starPainted(next), a.modelID, selected)
	}
	if err := tt.Click("Add " + rows[next].model.Label + " to favorites"); err != nil {
		t.Fatal(err)
	}
	if !a.isModelFavorite(rows[next].model) || a.modelID != selected || !a.modelListPage {
		t.Fatal("the favorite button selected the model or failed to persist its favorite")
	}
	if _, ok := tt.Find("Remove " + rows[next].model.Label + " from favorites"); !ok {
		t.Fatal("the starred row did not expose its remove control")
	}
	tt.Move(1200, 100)
	tt.Key(0, ui.KeyEscape)
	tt.Key(0, ui.KeyEnter)
	if !tt.Focused("Search models") {
		t.Fatal("returning to the list did not remount and refocus the search")
	}
	tt.Advance(600 * time.Millisecond)
	if !tt.Focused("Search models") {
		t.Fatal("native caret blink timing changed search focus")
	}
	tt.Type("GPT-5.3")
	if a.modelQuery != "GPT-5.3" {
		t.Fatal("the focused list search did not receive native text input")
	}
}

func TestCompactModelPanelKeyboardOpensCurrentOrNeighborModel(t *testing.T) {
	for _, tc := range []struct {
		name  string
		key   ui.Key
		delta int
	}{
		{"up", ui.KeyUp, -1},
		{"down", ui.KeyDown, 1},
		{"enter", ui.KeyEnter, 0},
		{"space", ui.KeySpace, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := testApp(t, "default")
			a.state.Settings.ModelPicker.Favorites = nil
			a = modelSnapshotApp(t, a.state)
			tt := ui.NewTester(a.View, 1320, 880)
			if err := tt.Click("Select model"); err != nil {
				t.Fatal(err)
			}
			rows := a.modelRows()
			selected := 0
			for i, row := range rows {
				if row.model.ID == a.modelID {
					selected = i
					break
				}
			}
			want := (selected + tc.delta + len(rows)) % len(rows)
			before := a.modelID
			tt.Key(0, tc.key)
			if !a.modelMenu || !a.modelListPage || a.modelCursor != want || a.modelID != before {
				t.Fatalf("panel key did not open the expected row: open=%v list=%v cursor=%d want=%d model=%s", a.modelMenu, a.modelListPage, a.modelCursor, want, a.modelID)
			}
			tt.Key(0, ui.KeyEnter)
			if a.modelID != rows[want].model.ID || !a.modelMenu || a.modelListPage {
				t.Fatal("Enter did not commit the highlighted model and return to the panel")
			}
			tt.Key(0, ui.KeyEscape)
			tt.Frame()
			if a.modelMenu {
				t.Fatal("Escape did not close the returned panel")
			}
		})
	}
}

func TestCompactModelPanelKeyboardAdjustsEffortWithoutSliderFocus(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 1320, 880)
	if err := tt.Click("Select model"); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		key  ui.Key
		want string
		stop int
	}{{ui.KeyEnd, "Max", 3}, {ui.KeyLeft, "High", 2}, {ui.KeyHome, "Low", 0}, {ui.KeyRight, "Medium", 1}} {
		tt.Key(0, step.key)
		if a.reasoning != step.want || a.effortValue != float64(step.stop) || a.modelListPage || !a.modelMenu {
			t.Fatalf("panel effort key %v selected %q, want %q", step.key, a.reasoning, step.want)
		}
	}
	if err := tt.Click("Reasoning effort"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyDown)
	if !a.modelListPage {
		t.Fatal("clicking the slider trapped Down in its native effort keys")
	}
}

func TestCompactModelPanelTabCyclesAvailableProvidersAtRememberedModel(t *testing.T) {
	a := testApp(t, "default")
	a.state.Settings.ModelPicker.Selections = map[string]contracts.ModelSelection{
		"openai": {ModelID: "gpt-5.3-codex", Reasoning: "Extra high"},
	}
	a = modelSnapshotApp(t, a.state)
	tt := ui.NewTester(a.View, 1320, 880)
	if err := tt.Click("Select model"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyTab)
	if a.modelID != "gpt-5.3-codex" || a.reasoning != "Extra high" || a.modelListPage {
		t.Fatalf("Tab did not use provider memory: %s / %s", a.modelID, a.reasoning)
	}
	tt.Key(ui.Shift, ui.KeyTab)
	if a.modelID != "claude-opus-4-6" {
		t.Fatal("Shift+Tab did not cycle backwards")
	}
	tt.Key(ui.Shift, ui.KeyTab)
	if a.modelID != "gpt-5.3-codex" {
		t.Fatal("provider cycling did not wrap past unavailable agents")
	}
}

func TestCompactFastModeUsesSourceOptionEncodingsAndKeepsOtherOptions(t *testing.T) {
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
				t.Fatalf("compact option controls did not follow the source encoding: %v", tt.Texts())
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
			a.modelMenu = false
			a.state.Settings.General.CompactModelPicker = false
			tt.Frame()
			if err := tt.Click("Select model"); err != nil {
				t.Fatal(err)
			}
			if !tt.HasText("Candidate preference") {
				t.Fatal("compact fast-mode filtering removed the standard picker option")
			}
		})
	}
}

func TestCompactOptionEffortUsesSliderAndKeepsOtherOptionsInteractive(t *testing.T) {
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
			if err := tt.Click("Select model"); err != nil {
				t.Fatal(err)
			}
			if _, ok := tt.Find("Reasoning effort"); !ok || tt.HasText("Option effort") {
				t.Fatal("option-backed effort was duplicated instead of using the compact slider")
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
			if err := tt.Click("200k"); err != nil {
				t.Fatal(err)
			}
			if err := tt.ChooseMenuItem("1m"); err != nil {
				t.Fatal(err)
			}
			if a.state.Settings.ModelPicker.Options[modelID+"/contextWindow"] != "1m" || !a.modelMenu {
				t.Fatal("another compact option stopped responding after keyboard effort changes")
			}
		})
	}
}

func TestModelOptionChoiceLabelsDisplayAndPersistSeparateIDs(t *testing.T) {
	for _, compact := range []bool{true, false} {
		name := "standard"
		if compact {
			name = "compact"
		}
		t.Run(name, func(t *testing.T) {
			for _, labels := range []bool{true, false} {
				name := "labels"
				if !labels {
					name = "legacy ids"
				}
				t.Run(name, func(t *testing.T) {
					a := testApp(t, "default")
					a.state.Settings.General.CompactModelPicker = compact
					modelID := a.modelID
					option := contracts.ModelOption{ID: "contextWindow", Label: "Context window", Choices: []string{"standard", "1m"}, DefaultChoice: "standard"}
					initial, next := "standard", "1m"
					if labels {
						option.ChoiceLabels = map[string]string{"standard": "Standard", "1m": "1M tokens"}
						initial, next = "Standard", "1M tokens"
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
					if err := tt.Click(initial); err != nil {
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
		})
	}
}

func TestCompactOptionEffortUsesLabelsWithoutSavingLabels(t *testing.T) {
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
	if err := tt.Click("Select model"); err != nil {
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

func TestSideCompactPanelKeyboardPreservesOwnershipAndProviderLock(t *testing.T) {
	a := testApp(t, "transcript")
	child := sideSession(t, a, "showcase-agent")
	for i := range a.state.Models {
		if a.state.Models[i].ID == child.ModelID {
			a.state.Models[i].Options = []contracts.ModelOption{{ID: "fastMode", Label: "Fast preference", Choices: []string{"off", "on"}, DefaultChoice: "off"}}
		}
	}
	for i := range a.state.Sessions {
		if a.state.Sessions[i].ID == child.ID {
			a.state.Sessions[i].DeviceID = "macbook"
		}
	}
	a.state.Integrations.Providers = []contracts.ProviderDetails{
		{TargetID: "local", ProviderID: "claude", Installed: true, Enabled: true},
		{TargetID: "macbook", ProviderID: "claude", Installed: true},
		{TargetID: "macbook", ProviderID: "openai", Installed: true, Enabled: true},
	}
	a = modelSnapshotApp(t, a.state)
	a.selectSession("chat-0")
	a.workspace.OpenSession(child.ID)
	recorder := &sideRecordingService{Service: a.service}
	a.service = recorder
	tt := ui.NewTester(a.View, 1600, 1000)
	if err := tt.Click("Select model in " + child.Title); err != nil {
		t.Fatal(err)
	}
	mainModel, mainReason := a.modelID, a.reasoning
	state := a.sideComposer(child.ID)
	tt.Key(0, ui.KeyHome)
	if state.reasoning != "Low" || a.reasoning != mainReason {
		t.Fatalf("side effort shortcut lost ownership: %s / %s", state.reasoning, a.reasoning)
	}
	tt.Key(0, ui.KeyF)
	if a.state.Settings.ModelPicker.Options[child.ModelID+"/fastMode"] != "on" || a.state.Settings.ModelPicker.Options[mainModel+"/fastMode"] != "" {
		t.Fatal("side fast-mode shortcut changed the wrong model's shared preference")
	}
	tt.Key(0, ui.KeyTab)
	if state.modelID != "gpt-5.4" || a.modelID != mainModel {
		t.Fatalf("side Tab borrowed local flags or changed main: %s / %s", state.modelID, a.modelID)
	}
	for i := range a.state.Sessions {
		if a.state.Sessions[i].ID == child.ID {
			a.state.Sessions[i].Status = contracts.Working
		}
	}
	tt.Frame()
	tt.Key(ui.Shift, ui.KeyTab)
	if state.modelID != "gpt-5.4" {
		t.Fatal("provider keyboard cycling bypassed the running side lock")
	}
	settingsWritten, sessionWritten := false, false
	for _, command := range recorder.commands {
		if command.Kind == "settings.update" && command.Settings != nil && command.Settings.ModelPicker.Options[child.ModelID+"/fastMode"] == "on" {
			settingsWritten = true
		}
		if command.Kind == "session.update" && command.Session != nil {
			if command.Session.ID != child.ID {
				t.Fatal("side keyboard dispatched a configuration update for another session")
			}
			sessionWritten = true
		}
	}
	if !settingsWritten || !sessionWritten {
		t.Fatal("keyboard changes bypassed the typed settings/session commands")
	}
}
