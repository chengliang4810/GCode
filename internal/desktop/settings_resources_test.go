package desktop

import (
	"context"
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"gcode/internal/contracts"
	localize "gcode/internal/i18n/view"
	"github.com/egoist/mygo/ui"
)

func TestResourceDraftParsingRejectsInvalidJSON(t *testing.T) {
	for _, value := range []string{"null", "[]", `{"key":true}`, `{"key":"value"} trailing`} {
		t.Run("object-"+value, func(t *testing.T) {
			if _, err := parseStringMap(value); err == nil {
				t.Fatal("invalid object accepted")
			}
		})
	}
	for _, value := range []string{"null", `{"key":"value"}`, `["ok",false]`, `["a"] []`} {
		t.Run("args-"+value, func(t *testing.T) {
			if _, err := parseStringArgs(value); err == nil {
				t.Fatal("invalid arguments accepted")
			}
		})
	}
	args, err := parseStringArgs(`["--path","C:\\directory with spaces",""]`)
	if err != nil || len(args) != 3 || args[1] != `C:\directory with spaces` || args[2] != "" {
		t.Fatalf("lossy arguments: %#v %v", args, err)
	}
}

func TestSettingsFormsReplaceListAndReturnWithoutLosingDraft(t *testing.T) {
	for _, test := range []struct {
		section, add, field, edit, editTitle string
		draw                                 func(*App, *ui.Context)
		fill                                 func(*App)
	}{
		{"Subagents", "Add subagent", "Subagent name", "Edit subagent Navigator", "Edit subagent", (*App).subagentSettings, func(app *App) {
			app.runtimeUI.subagent.Name = "Navigator"
			app.runtimeUI.subagent.Description = "Exercise form navigation"
			app.runtimeUI.subagent.Prompt = "Inspect the supplied files."
		}},
		{"MCP servers", "Add MCP server", "Server name", "Edit navigator", "Edit MCP server", (*App).mcpSettings, func(app *App) {
			app.capabilityUI.mcp.Name = "navigator"
			app.capabilityUI.mcp.Command = "node"
		}},
		{"Commands", "Add command", "Command name", "Edit /navigator", "Edit command", (*App).commandSettings, func(app *App) {
			app.capabilityUI.command.Name = "navigator"
			app.capabilityUI.command.Content = "Inspect $ARGUMENTS."
		}},
		{"Hooks", "Add hook", "Hook name", "Edit Navigator", "Edit hook", (*App).hookSettings, func(app *App) {
			app.capabilityUI.hook.Name = "Navigator"
			app.capabilityUI.hook.Command = "echo inspected"
		}},
	} {
		t.Run(test.section, func(t *testing.T) {
			app, _ := runtimeSettingsTestApp(t)
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(app.palette.UI())
				ui.Column(c).Fill().Padding(16).Gap(12).Children(func() { test.draw(app, c) })
			}, 1000, 1200)
			click := func(label string) {
				t.Helper()
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
				tt.Frame()
			}
			assertList := func() {
				t.Helper()
				if _, ok := tt.Find("Search " + test.section); !ok {
					t.Fatal("return did not restore the list toolbar")
				}
				if _, ok := tt.Find(test.field); ok {
					t.Fatal("closed form remains in the list")
				}
			}
			assertForm := func() {
				t.Helper()
				if _, ok := tt.Find("Search " + test.section); ok {
					t.Fatal("form was appended below the list toolbar")
				}
				if _, ok := tt.Find("Refresh"); ok {
					t.Fatal("list refresh action remains in the form")
				}
				if _, ok := tt.Find(test.field); !ok || !tt.HasText(test.add) {
					t.Fatal("form is missing its fields or create title")
				}
			}
			assertList()
			click(test.add)
			assertForm()
			test.fill(app)
			tt.Frame()
			click("Back to list")
			assertList()
			click(test.add)
			assertForm()
			click("Cancel")
			assertList()
			click(test.add)
			test.fill(app)
			tt.Frame()
			click("Save")
			if app.errorText != "" {
				t.Fatal(app.errorText)
			}
			assertList()
			click(test.edit)
			if _, ok := tt.Find("Search " + test.section); ok || !tt.HasText(test.editTitle) {
				t.Fatal("existing row did not open the full edit page")
			}
			click("Back to list")
			assertList()
		})
	}
}

func TestUserSettingsResourceCRUDWithRealCore(t *testing.T) {
	app, core := runtimeSettingsTestApp(t)
	page := "Commands"
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Column(c).FillWidth().Padding(16).Gap(12).Children(func() {
			switch page {
			case "Commands":
				app.commandSettings(c)
			case "Skills":
				app.skillSettings(c)
			case "MCP servers":
				app.mcpSettings(c)
			case "Hooks":
				app.hookSettings(c)
			}
		})
		app.settingsRemovalDialog(c)
	}, 900, 1100)
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if app.errorText != "" {
			t.Fatalf("%s: %s", label, app.errorText)
		}
	}
	snapshot := func() contracts.Snapshot {
		t.Helper()
		s, err := core.Snapshot(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	click("Add command")
	app.capabilityUI.command.Name = "review-change"
	app.capabilityUI.command.Description = "Review files"
	app.capabilityUI.command.Content = "Review $ARGUMENTS"
	app.capabilityUI.command.ArgumentHint = "<files>"
	tt.Frame()
	click("Save")
	if got := snapshot().UserResources.Commands; len(got) != 1 || got[0].Name != "review-change" || got[0].ArgumentHint != "<files>" {
		t.Fatalf("command not saved: %+v", got)
	}
	click("Enable /review-change")
	if snapshot().UserResources.Commands[0].Enabled {
		t.Fatal("command toggle not saved")
	}
	click("Edit /review-change")
	app.capabilityUI.command.Content = "Inspect $ARGUMENTS carefully"
	tt.Frame()
	click("Save")
	if snapshot().UserResources.Commands[0].Content != "Inspect $ARGUMENTS carefully" {
		t.Fatal("command edit not saved")
	}
	click("Remove /review-change")
	click("Confirm delete")
	if len(snapshot().UserResources.Commands) != 0 {
		t.Fatal("command delete not saved")
	}

	page = "Skills"
	tt.Frame()
	click("Add skill")
	app.capabilityUI.skill.Name = "Review skill"
	app.capabilityUI.skill.Description = "Review before release"
	app.capabilityUI.skill.Content = "# Review\nInspect the working tree."
	tt.Frame()
	click("Save")
	if got := snapshot().UserResources.Skills; len(got) != 1 || got[0].Name != "Review skill" {
		t.Fatalf("skill not saved: %+v", got)
	}
	click("Enable Review skill")
	if snapshot().UserResources.Skills[0].Enabled {
		t.Fatal("skill toggle not saved")
	}
	click("Remove Review skill")
	click("Confirm delete")
	if len(snapshot().UserResources.Skills) != 0 {
		t.Fatal("skill delete not saved")
	}
	importDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(importDirectory, "SKILL.md"), []byte("---\nname: imported-skill\ndescription: Imported instructions\n---\n\nRead examples before making changes.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(importDirectory, "examples"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(importDirectory, "examples", "usage.md"), []byte("Example usage."), 0644); err != nil {
		t.Fatal(err)
	}
	click("Import skill")
	app.capabilityUI.skillImport = importDirectory
	app.capabilityUI.skill.Name = ""
	app.capabilityUI.skill.Description = ""
	tt.Frame()
	click("Import")
	if got := snapshot().UserResources.Skills; len(got) != 1 || got[0].Name != "imported-skill" || !slices.Contains(got[0].Files, "examples/usage.md") {
		t.Fatalf("skill directory not imported: %+v", got)
	}

	page = "MCP servers"
	tt.Frame()
	click("Add MCP server")
	app.capabilityUI.mcp.Name = "localtools"
	app.capabilityUI.mcp.Command = "node"
	app.capabilityUI.mcpArgs = `["server.js","space value"]`
	app.capabilityUI.mcpEnv = `{"TOKEN":"very-secret"}`
	tt.Frame()
	click("Save")
	if got := snapshot().UserResources.MCPServers; len(got) != 1 || got[0].Name != "localtools" || !slices.Equal(got[0].Args, []string{"server.js", "space value"}) || !slices.Equal(got[0].EnvKeys, []string{"TOKEN"}) {
		t.Fatalf("MCP not saved: %+v", got)
	}
	click("Edit localtools")
	if app.capabilityUI.mcpEnv != "" {
		t.Fatal("saved credentials leaked into editor")
	}
	click("Edit JSON")
	if app.capabilityUI.mcpJSON == "" || tt.HasText("very-secret") {
		t.Fatal("invalid JSON editor or leaked secret")
	}
	click("Edit JSON")
	click("Cancel")
	click("Enable localtools")
	if snapshot().UserResources.MCPServers[0].Enabled {
		t.Fatal("MCP toggle not saved")
	}
	click("Remove localtools")
	click("Confirm delete")
	if len(snapshot().UserResources.MCPServers) != 0 {
		t.Fatal("MCP delete not saved")
	}

	page = "Hooks"
	tt.Frame()
	click("Add hook")
	app.capabilityUI.hook.Name = "Audit"
	app.capabilityUI.hook.Command = "echo audit"
	app.capabilityUI.hookTimeout = "0.25"
	tt.Frame()
	click("Hook event")
	click("Hook event PostToolUse")
	click("Save")
	if got := snapshot().Settings.Hooks; len(got) != 1 || got[0].Event != contracts.HookPostToolUse || got[0].TimeoutSeconds != 0.25 {
		t.Fatalf("hook not saved: %+v", got)
	}
	click("Enable Audit")
	if snapshot().Settings.Hooks[0].Enabled {
		t.Fatal("hook toggle not saved")
	}
	click("Remove Audit")
	click("Confirm delete")
	if len(snapshot().Settings.Hooks) != 0 {
		t.Fatal("hook delete not saved")
	}
}

func TestResourceSaveFailureRetainsCurrentDraft(t *testing.T) {
	app, _ := runtimeSettingsTestApp(t)
	app.editMCP(nil)
	app.capabilityUI.mcp.Name = "invalid name"
	app.capabilityUI.mcp.Command = "node"
	app.capabilityUI.mcpEnv = `{"TOKEN":"retry-secret"}`
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(app.palette.UI()); app.mcpSettings(c) }, 900, 1100)
	if err := tt.Click("Save"); err != nil {
		t.Fatal(err)
	}
	if app.errorText == "" || !app.capabilityUI.mcpOpen || app.capabilityUI.mcp.Name != "invalid name" || app.capabilityUI.mcpEnv != `{"TOKEN":"retry-secret"}` {
		t.Fatalf("failed draft lost: %+v %s", app.capabilityUI, app.errorText)
	}
	old := contracts.Command{Kind: "resource.mcp.save", Resource: &contracts.ResourceInput{MCP: &contracts.MCPServerInput{ID: app.capabilityUI.mcp.ID}}}
	app.editMCP(nil)
	id := app.capabilityUI.mcp.ID
	app.capabilityCommandFinished(old, context.Canceled)
	if app.capabilityUI.mcp.ID != id {
		t.Fatal("late failure replaced a new draft")
	}
}

func TestMCPFullPageJSONValidationRetainsEditor(t *testing.T) {
	app, _ := runtimeSettingsTestApp(t)
	app.editMCP(nil)
	app.capabilityUI.mcp.Name = "json-editor"
	app.capabilityUI.mcp.Command = "node"
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Column(c).Fill().Padding(16).Gap(12).Children(func() { app.mcpSettings(c) })
	}, 1000, 1000)
	if err := tt.Click("Edit JSON"); err != nil {
		t.Fatal(err)
	}
	app.capabilityUI.mcpJSON = `{"name":`
	tt.Frame()
	for _, action := range []string{"Edit JSON", "Save"} {
		app.errorText = ""
		if err := tt.Click(action); err != nil {
			t.Fatal(err)
		}
		if app.errorText == "" || !app.capabilityUI.mcpOpen || !app.capabilityUI.mcpJSONMode || app.capabilityUI.mcpJSON != `{"name":` {
			t.Fatalf("%s discarded the invalid JSON draft", action)
		}
		if _, ok := tt.Find("Search MCP servers"); ok {
			t.Fatal("invalid draft returned to the list")
		}
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("Search MCP servers"); !ok {
		t.Fatal("cancel did not return to the list")
	}
}

func TestSubagentUserControlsPersist(t *testing.T) {
	app, core := runtimeSettingsTestApp(t)
	profile := app.state.Settings.Subagents[0]
	app.editSubagent(&profile)
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(app.palette.UI()); app.subagentSettings(c) }, 1000, 1200)
	for _, label := range []string{"Use project instructions", "All default tools", "Available tools Read"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	if err := tt.Click("Save"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := core.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := snapshot.Settings.Subagents[0]
	if !got.SkipProjectInstructions || !slices.Equal(got.Tools, []string{"ListDirectory", "Glob", "Grep"}) {
		t.Fatalf("subagent controls not saved: %+v", got)
	}
	if err := tt.Click("Enable " + profile.Name); err != nil {
		t.Fatal(err)
	}
	snapshot, err = core.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.Settings.Subagents[0].Disabled {
		t.Fatal("subagent disable not saved")
	}
	explore := snapshot.Settings.Subagents[1]
	app.editSubagent(&explore)
	tt.Frame()
	if _, ok := tt.Find("Available tools Bash"); ok {
		t.Fatal("read-only profile exposes shell permission")
	}
}

func TestSettingsEditorSaveFailureRetainsDraft(t *testing.T) {
	for _, page := range []string{"Hooks", "Subagents"} {
		t.Run(page, func(t *testing.T) {
			app, _ := runtimeSettingsTestApp(t)
			app.service = failingLanguageService{app.service}
			if page == "Hooks" {
				app.editHook(nil)
				app.capabilityUI.hook.Name = "Keep this hook"
				app.capabilityUI.hook.Command = "echo retained"
			} else {
				app.editSubagent(nil)
				app.runtimeUI.subagent.Name = "Keep this agent"
				app.runtimeUI.subagent.Description = "Retain failed settings"
				app.runtimeUI.subagent.Prompt = "Do not lose this draft."
			}
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(app.palette.UI())
				if page == "Hooks" {
					app.hookSettings(c)
				} else {
					app.subagentSettings(c)
				}
			}, 1000, 1200)
			if err := tt.Click("Save"); err != nil {
				t.Fatal(err)
			}
			if app.errorText == "" {
				t.Fatal("settings failure not shown")
			}
			if page == "Hooks" {
				if !app.capabilityUI.hookOpen || app.capabilityUI.hook.Name != "Keep this hook" || app.capabilityUI.hook.Command != "echo retained" {
					t.Fatal("hook draft lost after failure")
				}
				if len(app.state.Settings.Hooks) != 0 {
					t.Fatal("failed hook projected as persisted")
				}
			} else {
				if !app.runtimeUI.subagentOpen || app.runtimeUI.subagent.Name != "Keep this agent" || app.runtimeUI.subagent.Prompt != "Do not lose this draft." {
					t.Fatal("subagent draft lost after failure")
				}
				if slices.ContainsFunc(app.state.Settings.Subagents, func(profile contracts.SubagentProfile) bool { return profile.Name == "Keep this agent" }) {
					t.Fatal("failed profile projected as persisted")
				}
			}
		})
	}
}

func TestUserResourceSettingsRenderBothLanguages(t *testing.T) {
	for _, language := range []string{"en", "zh-CN"} {
		t.Run(language, func(t *testing.T) {
			app, _ := runtimeSettingsTestApp(t)
			app.state.Settings.General.Language = language
			app.route = "settings"
			app.section = "MCP servers"
			app.state.UserResources.MCPServers = []contracts.MCPServer{{ID: "sample", Name: "localtools", Command: "node", Args: []string{"server.js"}, Transport: "stdio", Enabled: true, Test: contracts.MCPTestResult{State: "ready", Tools: []contracts.MCPToolSummary{{Name: "lookup"}}}}}
			app.editMCP(&app.state.UserResources.MCPServers[0])
			tt := ui.NewTester(func(c *ui.Context) {
				localize.Set(c, language)
				c.SetTheme(app.palette.UI())
				ui.Row(c).Fill().Children(func() {
					ui.Column(c).Width(220).FillHeight().Children(func() { app.settingsNavigation(c) })
					app.settingsView(c)
				})
			}, 1100, 940)
			for _, label := range []string{"Model settings", "Subagents", "MCP servers", "Skills", "Commands", "Hooks"} {
				if _, ok := tt.Find(app.t(label)); !ok {
					t.Errorf("missing user settings navigation %s", label)
				}
			}
			if tt.HasText("Project") || tt.HasText("项目级") {
				t.Fatal("unexpected project settings")
			}
			directory := os.Getenv("RCODE_SETTINGS_EVIDENCE")
			if directory == "" {
				return
			}
			if err := os.MkdirAll(directory, 0755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(directory, "settings-resources-"+language)
			file, err := os.Create(path + ".png")
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(file, tt.Image()); err != nil {
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			data, _ := json.MarshalIndent(map[string]any{"renderer": "MyGo ui.NewTester; not native-window or visual parity acceptance", "language": language, "width": 1100, "height": 940}, "", "  ")
			if err := os.WriteFile(path+".json", append(data, '\n'), 0644); err != nil {
				t.Fatal(err)
			}
		})
	}
}
