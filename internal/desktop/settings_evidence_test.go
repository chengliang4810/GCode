package desktop

import (
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"gcode/internal/contracts"
	localize "gcode/internal/i18n/view"
	"gcode/internal/model"
	"github.com/egoist/mygo/ui"
)

func TestUserSettingsSixPageRenderEvidence(t *testing.T) {
	for _, language := range []string{"en", "zh-CN"} {
		for _, page := range []struct{ name, section string }{
			{"models", "Model settings"}, {"models-editor", "Model settings"}, {"subagents", "Subagents"},
			{"skills", "Skills"}, {"commands", "Commands"}, {"hooks", "Hooks"}, {"mcp", "MCP servers"},
			{"model-metadata", "Model settings"}, {"skill-details", "Skills"}, {"skill-import", "Skills"},
			{"mcp-import", "MCP servers"}, {"command-import", "Commands"}, {"removal-confirm", "Model settings"},
		} {
			t.Run(page.name+"-"+language, func(t *testing.T) {
				app, _ := runtimeSettingsTestApp(t)
				app.state.Settings.General.Language = language
				app.route, app.section = "settings", page.section
				fixture := model.Provider{ID: "fixture-provider", Name: "Development gateway", Protocol: model.Responses, BaseURL: "https://example.com/v1",
					Models: []model.Model{{ID: "deep-model", ContextWindow: 200000, MaxOutput: 16000, ReasoningLevels: []string{"low", "high", "max"}}, {ID: "quick-model", ContextWindow: 32000, MaxOutput: 4000}}}
				app.state.CustomProviders = []contracts.ProviderConfiguration{{Provider: fixture}}
				app.state.UserResources = contracts.UserResourceState{
					Skills:     []contracts.UserSkill{{ID: "release-review", Name: "Release review", Description: "Review tests and the working tree before release.", Content: "# Release review\n\nInspect the changed files, run relevant checks and report unresolved risks.", Enabled: true, Files: []string{"SKILL.md", "examples/release-checklist.md"}}},
					Commands:   []contracts.UserCommand{{ID: "review", Name: "review", Description: "Review a focused change.", ArgumentHint: "<files>", Content: "Review $ARGUMENTS. Read the changed files and return actionable findings with file references.", Enabled: true}},
					MCPServers: []contracts.MCPServer{{ID: "sample", Name: "localtools", Command: "node", Args: []string{"server.js"}, Transport: "stdio", Enabled: true, Test: contracts.MCPTestResult{State: "ready", Tools: []contracts.MCPToolSummary{{Name: "lookup", Description: "Look up an item by its identifier."}}}}},
				}
				app.state.Settings.Hooks = []contracts.HookRule{{ID: "audit", Name: "Tool audit", Event: contracts.HookPostToolUse, Matcher: "Write|Edit", Type: "command", Command: "audit-helper", Args: []string{"--record"}, TimeoutSeconds: 0.25, Enabled: true}}
				switch page.name {
				case "models-editor":
					app.modelsUI.selected = fixture.ID
					app.syncInlineProvider(app.state.CustomProviders[0])
					app.runtimeUI.provider.Name = "New gateway"
					app.runtimeUI.provider.BaseURL = "https://example.com/v1"
					app.runtimeUI.provider.Models = []model.Model{fixture.Models[0]}
					app.markInlineProviderDirty()
				case "subagents":
					app.editSubagent(&app.state.Settings.Subagents[0])
				case "skills":
					app.editSkill(&app.state.UserResources.Skills[0])
				case "commands":
					app.editCommand(&app.state.UserResources.Commands[0])
				case "hooks":
					app.editHook(&app.state.Settings.Hooks[0])
				case "mcp":
					app.editMCP(&app.state.UserResources.MCPServers[0])
				case "model-metadata":
					app.openSavedModel(fixture.ID, "deep-model")
				case "skill-details":
					app.showSkillDetails(app.state.UserResources.Skills[0])
				case "skill-import":
					app.importSkill()
					app.capabilityUI.skillImport = `D:\skills\release-review`
				case "mcp-import":
					app.openSettingsImport("mcp")
				case "command-import":
					app.openSettingsImport("command")
				case "removal-confirm":
					app.confirmSettingsRemoval("provider", fixture.ID, fixture.Name)
				}
				tt := ui.NewTester(func(c *ui.Context) {
					localize.Set(c, language)
					c.SetTheme(app.palette.UI())
					c.Root().Font("Geist").FontSize(13).Background(app.palette.Background).TextColor(app.palette.Text)
					ui.Row(c).Fill().Children(func() {
						ui.Column(c).Width(220).FillHeight().Children(func() { app.settingsNavigation(c) })
						app.settingsView(c)
					})
				}, 1100, 1100)
				for _, label := range []string{"Model settings", "Subagents", "MCP servers", "Skills", "Commands", "Hooks"} {
					r, ok := tt.Find(app.t(label))
					if !ok || r.X < 0 || r.X+r.W > 220 {
						t.Fatalf("missing or clipped navigation: %s %+v", label, r)
					}
				}
				if page.name != "models" {
					action := "Save"
					switch page.name {
					case "models-editor":
						action = "Add model"
					case "model-metadata":
						action = "Save model"
					case "skill-details":
						action = "Edit skill"
					case "skill-import", "mcp-import", "command-import":
						action = "Import"
					case "removal-confirm":
						action = "Confirm delete"
					}
					r, ok := tt.Find(app.t(action))
					if !ok || r.X < 220 || r.X+r.W > 1100 || r.Y < 0 || r.Y+r.H > 1100 {
						t.Fatalf("editor Save is clipped: %+v", r)
					}
				}
				directory := os.Getenv("RCODE_SETTINGS_EVIDENCE")
				if directory == "" {
					return
				}
				if err := os.MkdirAll(directory, 0755); err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(directory, page.name+"-"+language)
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
				metadata, _ := json.MarshalIndent(map[string]any{"renderer": "MyGo ui.NewTester implementation render; not native-window or visual-parity acceptance", "fixture": true, "fixtureDetails": "User-owned provider, resources and hook snapshots are representative fixtures. No external provider or MCP connection is made by this render test.", "language": language, "page": page.section, "view": page.name, "width": 1100, "height": 1100}, "", "  ")
				if err := os.WriteFile(path+".json", append(metadata, '\n'), 0644); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
