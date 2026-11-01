package desktop

import (
	"context"
	"slices"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"github.com/egoist/mygo/ui"
)

func TestSubagentBuiltinsRemainVisibleWithoutSavedProfiles(t *testing.T) {
	app, core := runtimeSettingsTestApp(t)
	app.state.Settings.Subagents = nil
	app.saveSettings()
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.subagentSettings(c)
	}, 1000, 800)
	for _, name := range []string{"General purpose", "Explore"} {
		if !tt.HasText(name) {
			t.Errorf("built-in subagent %q is missing without saved overrides", name)
		}
		if _, present := tt.Find("Remove subagent " + name); present {
			t.Error("built-in profile has a delete action")
		}
	}
	if err := tt.Click("Enable General purpose"); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := core.Snapshot(context.Background())
	if len(snapshot.Settings.Subagents) != 1 || !snapshot.Settings.Subagents[0].Disabled {
		t.Fatal("disabling an unsaved built-in did not persist an explicit override")
	}
	if err := tt.Click("Edit subagent Explore"); err != nil {
		t.Fatal(err)
	}
	app.runtimeUI.subagent.Description = "Inspect the evidence."
	app.runtimeUI.subagent.Prompt = "Use read tools and report paths."
	tt.Frame()
	if err := tt.Click("Save"); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = core.Snapshot(context.Background())
	if len(snapshot.Settings.Subagents) != 2 || snapshot.Settings.Subagents[1].Prompt != "Use read tools and report paths." {
		t.Fatal("editing an unsaved built-in did not create its override")
	}
	app.state.Settings.General.Language = "zh-CN"
	app.route, app.section = "settings", "Subagents"
	tt = ui.NewTester(app.View, 1180, 900)
	for _, text := range []string{"通用智能体", "探索智能体", "内置"} {
		if !tt.HasText(text) {
			t.Errorf("missing localized built-in text %q", text)
		}
	}
}

func TestSubagentProfileModelSwitchAndInheritance(t *testing.T) {
	app, core := runtimeSettingsTestApp(t)
	provider := model.Provider{ID: "user", Name: "My provider", Protocol: model.Responses, BaseURL: "https://example.com/v1", Models: []model.Model{
		{ID: "first", ReasoningLevels: []string{"low", "high", "max"}},
		{ID: "second", ReasoningLevels: []string{"medium", "high", "xhigh"}},
		{ID: "plain"},
	}}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider}}); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := core.Snapshot(context.Background())
	app.apply(snapshot)
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.subagentSettings(c)
		app.settingsRemovalDialog(c)
	}, 900, 1000)
	if err := tt.Click("Add subagent"); err != nil {
		t.Fatal(err)
	}
	app.runtimeUI.subagent.Name, app.runtimeUI.subagent.Description, app.runtimeUI.subagent.Prompt = "Reviewer", "Review an independent change", "Inspect the actual files."
	tt.Frame()
	if !tt.HasText("Default main model") {
		t.Fatal("new profiles must inherit main model and reasoning")
	}
	choose := func(id string) {
		t.Helper()
		if err := tt.Click("Subagent model"); err != nil {
			t.Fatal(err)
		}
		if err := tt.Click("Subagent model " + id); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
	}
	choose("user/first")
	if app.runtimeUI.subagentReasoning != "Max" || !tt.HasText("My provider/first") {
		t.Fatal("selecting a model did not select its last configured level")
	}
	choose("user/second")
	if app.runtimeUI.subagentReasoning != "Extra high" {
		t.Fatal("switching models retained the previous model's reasoning")
	}
	choose("user/plain")
	if app.runtimeUI.subagentReasoning != "" {
		t.Fatal("no-reasoning model retained a reasoning level")
	}
	if _, ok := tt.Find("Subagent reasoning level"); ok {
		t.Fatal("no-reasoning model exposed a reasoning picker")
	}
	choose("user/first")
	if err := tt.Click("Save"); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = core.Snapshot(context.Background())
	index := slices.IndexFunc(snapshot.Settings.Subagents, func(profile contracts.SubagentProfile) bool { return profile.Name == "Reviewer" })
	if index < 0 || snapshot.Settings.Subagents[index].Model == nil || *snapshot.Settings.Subagents[index].Model != (contracts.ModelSelection{ModelID: "user/first", Reasoning: "Max"}) {
		t.Fatalf("model and reasoning were not saved together: %+v", snapshot.Settings.Subagents)
	}
	profile := snapshot.Settings.Subagents[index]
	app.editSubagent(&profile)
	tt.Frame()
	if err := tt.Click("Subagent model"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Default main model"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Save"); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = core.Snapshot(context.Background())
	if snapshot.Settings.Subagents[index].Model != nil {
		t.Fatal("inheritance persisted an explicit child model")
	}
	if err := tt.Click("Remove subagent Reviewer"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Confirm delete"); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = core.Snapshot(context.Background())
	if slices.ContainsFunc(snapshot.Settings.Subagents, func(candidate contracts.SubagentProfile) bool {
		return candidate.ID == profile.ID
	}) {
		t.Fatal("deleted subagent profile remained in settings")
	}
}

func TestSubagentUnavailableModelOrReasoningIsNotReplaced(t *testing.T) {
	for _, test := range []struct {
		name      string
		selection contracts.ModelSelection
		models    []contracts.Model
	}{
		{"missing model", contracts.ModelSelection{ModelID: "removed/model", Reasoning: "Max"}, nil},
		{"removed level", contracts.ModelSelection{ModelID: "user/model", Reasoning: "Max"}, []contracts.Model{{ID: "user/model", ProviderID: "user", Label: "Model", ReasoningLevels: []string{"Low", "High"}}}},
		{"removed capabilities", contracts.ModelSelection{ModelID: "user/model", Reasoning: "Max"}, []contracts.Model{{ID: "user/model", ProviderID: "user", Label: "Model"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			app, _ := runtimeSettingsTestApp(t)
			app.state.Models = test.models
			profile := contracts.SubagentProfile{ID: "test", Name: "Test", Description: "Test profile", Model: &test.selection}
			app.editSubagent(&profile)
			tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(app.palette.UI()); app.subagentSettings(c) }, 900, 1000)
			tt.Frame()
			if !tt.HasText("The configured model or reasoning level is unavailable.") || app.subagentSelectionValid() || app.runtimeUI.subagentReasoning != "Max" {
				t.Fatal("unavailable configuration was silently changed")
			}
			_ = tt.Click("Save")
			tt.Frame()
			if !app.runtimeUI.subagentOpen {
				t.Fatal("unavailable profile was saved")
			}
		})
	}
}
