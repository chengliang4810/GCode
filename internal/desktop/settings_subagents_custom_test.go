package desktop

import (
	"context"
	"errors"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func TestCustomSubagentToolsAreProgressiveAndValidateEmptySelection(t *testing.T) {
	app, core := runtimeSettingsTestApp(t)
	app.editSubagent(nil)
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Column(c).Fill().Padding(16).Children(func() { app.subagentSettings(c) })
	}, 1000, 1000)
	if _, present := tt.Find("Available tools Read"); present {
		t.Fatal("inherited tools expose unnecessary checkboxes")
	}
	if err := tt.Click("Save"); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"Enter a subagent name.", "Enter a subagent description.", "Enter a system prompt."} {
		if !tt.HasText(text) {
			t.Errorf("missing field-specific validation: %s", text)
		}
	}
	app.runtimeUI.subagent.Name, app.runtimeUI.subagent.Description, app.runtimeUI.subagent.Prompt = "Reader", "Read evidence", "Report the relevant paths."
	tt.Frame()
	chooseTools := func(label string) {
		t.Helper()
		if err := tt.Click("Tool access"); err != nil {
			t.Fatal(err)
		}
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
	}
	chooseTools("Custom tools")
	if !app.runtimeUI.subagentToolsCustom || !slices.Equal(app.runtimeUI.subagent.Tools, customSubagentTools) {
		t.Fatal("custom mode did not start with the visible default tools selected")
	}
	for _, tool := range customSubagentTools {
		if err := tt.Click("Available tools " + tool); err != nil {
			t.Fatal(err)
		}
	}
	if err := tt.Click("Save"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Select at least one tool.") || !app.runtimeUI.subagentOpen {
		t.Fatal("empty explicit tools were saved as unrestricted inheritance")
	}
	if err := tt.Click("Available tools Read"); err != nil {
		t.Fatal(err)
	}
	chooseTools("All default tools")
	if _, present := tt.Find("Available tools Read"); present {
		t.Fatal("switching back to default tools did not collapse the tool grid")
	}
	if err := tt.Click("Save"); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := core.Snapshot(context.Background())
	profile := snapshot.Settings.Subagents[len(snapshot.Settings.Subagents)-1]
	if app.runtimeUI.subagentOpen || profile.Name != "Reader" || len(profile.Tools) != 0 {
		t.Fatal("default tool mode did not persist inheritance and return to the list")
	}
}

func TestCustomSubagentPendingSaveAndFailureRetainDraft(t *testing.T) {
	app, core := runtimeSettingsTestApp(t)
	app.editSubagent(nil)
	app.runtimeUI.subagent.Name, app.runtimeUI.subagent.Description, app.runtimeUI.subagent.Prompt = "Queued", "Retain draft", "Investigate without losing this draft."
	app.service = failingLanguageService{core}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Column(c).Fill().Padding(16).Children(func() { app.subagentSettings(c) })
	}, 1000, 1000)
	if err := tt.Click("Save"); err != nil {
		t.Fatal(err)
	}
	if app.runtimeUI.subagentPendingID != "" || !app.runtimeUI.subagentOpen ||
		app.runtimeUI.subagent.Name != "Queued" || app.runtimeUI.subagentError == "" {
		t.Fatal("failed save lost the editor draft or error")
	}
	snapshot, _ := core.Snapshot(context.Background())
	if len(snapshot.Settings.Subagents) != len(contracts.DefaultSubagentProfiles()) {
		t.Fatal("failed save reached the real core")
	}
	// Component tests drive the pending projection without a fake native
	// Window. The real asynchronous event path is exercised by native E2E.
	app.runtimeUI.subagentPendingID = "retry"
	app.runtimeUI.subagentPendingProfile = app.runtimeUI.subagent.ID
	tt.Frame()
	if !tt.HasText("Saving…") {
		t.Fatal("pending save has no visible progress")
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	if !app.runtimeUI.subagentOpen {
		t.Fatal("Cancel discarded a draft while it was being saved")
	}
	app.customSubagentCommandFinished(contracts.Command{Kind: "settings.update", ID: "obsolete"}, errors.New("late failure"))
	if app.runtimeUI.subagentPendingID != "retry" {
		t.Fatal("obsolete completion settled a newer save")
	}
	app.runtimeUI.subagentPendingID = ""
	app.service = core
	tt.Frame()
	if err := tt.Click("Save"); err != nil {
		t.Fatal(err)
	}
	if app.runtimeUI.subagentOpen || !tt.HasText("Queued") || app.runtimeUI.subagentError != "" {
		t.Fatal("successful retry did not return to the saved list")
	}
}

func TestCustomSubagentEditorLayoutAcrossWindowsAndThemes(t *testing.T) {
	for _, test := range []struct {
		name, language string
		width          int
		dark           bool
	}{
		{name: "wide-light", language: "en", width: 1320},
		{name: "wide-dark", language: "zh-CN", width: 1320, dark: true},
		{name: "narrow-light", language: "zh-CN", width: 900},
		{name: "narrow-dark", language: "en", width: 900, dark: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			app, _ := runtimeSettingsTestApp(t)
			app.route, app.section = "settings", "Subagents"
			app.state.Settings.General.Language = test.language
			app.state.Settings.Appearance.Mode = "Light"
			if test.dark {
				app.state.Settings.Appearance.Mode = "Dark"
			}
			app.state.Providers = []contracts.Provider{{ID: "custom", Name: "Development"}}
			app.state.Models = []contracts.Model{{ID: "custom/model", ProviderID: "custom", Label: "Reasoning model", ReasoningLevels: []string{"Low", "High", "Max"}}}
			profile := contracts.SubagentProfile{ID: "reviewer", Name: "Code reviewer", Description: "Inspect the implementation and report actionable findings.",
				Prompt: "Read the changed files and verify the result.", Model: &contracts.ModelSelection{ModelID: "custom/model", Reasoning: "High"}}
			app.state.Settings.Subagents = append(app.state.Settings.Subagents, profile)
			app.editSubagent(&profile)
			tt := ui.NewTester(app.View, test.width, 1000)
			bounds := map[string]ui.Rect{}
			for _, key := range []string{"Subagent name", "Subagent model", "Subagent reasoning level", "Subagent description", "Tool access", "Subagent prompt", "Save", "Cancel"} {
				rect, ok := tt.Find(app.t(key))
				if !ok || rect.W <= 0 || rect.H <= 0 || rect.X < 256 ||
					rect.X+rect.W > float32(test.width) || rect.Y < 38 || rect.Y+rect.H > 1000 {
					t.Fatalf("%s is missing or clipped: %+v", key, rect)
				}
				bounds[key] = rect
			}
			name, chosen, reasoning := bounds["Subagent name"], bounds["Subagent model"], bounds["Subagent reasoning level"]
			aligned := func(a, b float32) bool { return math.Abs(float64(a-b)) < 1 }
			if test.width == 1320 && (!aligned(name.Y, chosen.Y) || !aligned(chosen.Y, reasoning.Y) || name.X >= chosen.X || chosen.X >= reasoning.X) {
				t.Fatalf("identity/model/reasoning are not aligned in the wide top row: %+v", bounds)
			}
			if bounds["Subagent description"].Y <= chosen.Y || bounds["Subagent prompt"].Y <= bounds["Tool access"].Y {
				t.Fatal("description/tools/prompt order does not follow the form hierarchy")
			}
			if directory := os.Getenv("RCODE_CUSTOM_SUBAGENT_EVIDENCE"); directory != "" {
				writeCustomSubagentImage(t, tt, directory, test.name+"-form")
				app.runtimeUI.subagentOpen = false
				tt.Frame()
				writeCustomSubagentImage(t, tt, directory, test.name+"-list")
			}
		})
	}
}

func writeCustomSubagentImage(t *testing.T, tt *ui.Tester, directory, name string) {
	t.Helper()
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(directory, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	err = png.Encode(file, tt.Image())
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("write custom subagent render: %v %v", err, closeErr)
	}
}
