package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/i18n"
	localize "gcode/internal/i18n/view"
	"github.com/egoist/mygo/ui"
)

type pluginUIService struct {
	contracts.Service
	state    contracts.PluginState
	commands []contracts.Command
}

func (s *pluginUIService) Execute(_ context.Context, command contracts.Command) error {
	if strings.HasPrefix(command.Kind, "plugins.") || command.Kind == "request.answer" {
		if err := command.Validate(); err != nil {
			return err
		}
		s.commands = append(s.commands, command.Clone())
		return nil
	}
	return nil
}

func (s *pluginUIService) Snapshot(ctx context.Context) (contracts.Snapshot, error) {
	snapshot, err := s.Service.Snapshot(ctx)
	snapshot.Plugins = s.state.Clone()
	return snapshot, err
}

func pluginUIFixture(t *testing.T) (*App, *pluginUIService) {
	t.Helper()
	app, _ := runtimeSettingsTestApp(t)
	state := contracts.PluginState{
		Marketplaces: []contracts.PluginMarketplace{
			{ID: "official", Name: "Official", Source: "owner/repository", Kind: "github", State: "ready", PluginCount: 1},
			{ID: "community", Name: "Community", Source: "owner/community", Kind: "github", State: "ready"},
		},
		Catalog: []contracts.PluginCatalogItem{
			{ID: "fixture@official", Name: "fixture", Marketplace: "official", Description: "Fixture search skill", Components: []string{"skills"}, Issues: []string{"Fixture dependency issue"}},
		},
		Installed: []contracts.InstalledPlugin{
			{ID: "fixture@official", CatalogID: "fixture@official", Name: "fixture", Marketplace: "official", Enabled: true,
				Config: []contracts.PluginConfigField{{Key: "token", Title: "Plugin token", Type: "string", Sensitive: true, Configured: true, Saved: true}}},
		},
	}
	service := &pluginUIService{Service: app.service, state: state}
	app.service, app.state.Plugins = service, state.Clone()
	app.state.Projects = []contracts.Project{{ID: "project-id", Name: "Project fixture", Path: t.TempDir()}}
	return app, service
}

func pluginUITester(app *App) *ui.Tester {
	if app.section != "Marketplace" {
		app.section = "Plugins"
	}
	return ui.NewTester(func(c *ui.Context) {
		localize.Set(c, app.state.Settings.General.Language)
		c.SetTheme(app.palette.UI())
		app.settingsView(c)
	}, 900, 1200)
}

func TestPluginSettingsNavigationAndEmptyState(t *testing.T) {
	app, _ := pluginUIFixture(t)
	sections := app.sections()
	models := slices.Index(sections, "Model settings")
	if models < 0 || sections[models+1] != "Marketplace" || !slices.Contains(sections, "Plugins") {
		t.Fatalf("marketplace must immediately follow model settings: %v", sections)
	}
	app.state.Plugins.Installed = nil
	tt := pluginUITester(app)
	if !tt.HasText("No installed plugins") {
		t.Fatal("empty installed state missing")
	}
	if _, ok := tt.Find("Search plugins"); ok {
		t.Fatal("installed page displays marketplace catalog")
	}
	if err := tt.Click("Browse plugin marketplace"); err != nil {
		t.Fatal(err)
	}
	if app.section != "Marketplace" {
		t.Fatal("empty state did not navigate to marketplace")
	}
	if _, ok := tt.Find("Search plugins"); !ok {
		t.Fatal("marketplace search missing after navigation")
	}
}

func TestPluginSettingsInstallUsesUserRegistry(t *testing.T) {
	app, service := pluginUIFixture(t)
	app.section = "Marketplace"
	app.state.Plugins.Installed = nil
	tt := pluginUITester(app)
	for _, removed := range []string{"Installation scope", "Project directory", "Marketplace"} {
		if _, ok := tt.Find(removed); ok {
			t.Fatalf("obsolete selector present: %s", removed)
		}
	}
	if err := tt.Click("Install fixture"); err != nil {
		t.Fatal(err)
	}
	if len(service.commands) != 1 || service.commands[0].Kind != "plugins.install" ||
		*service.commands[0].Plugin.Install != (contracts.PluginInstallInput{Marketplace: "official", Name: "fixture"}) {
		t.Fatalf("expected user installation command: %#v; %s", service.commands, app.errorText)
	}
}

func TestPluginSettingsFiltersAndSources(t *testing.T) {
	app, service := pluginUIFixture(t)
	app.section = "Marketplace"
	app.state.Plugins.Installed = nil
	tt := pluginUITester(app)
	all, ok := tt.Find("All marketplaces")
	official, found := tt.Find("Official")
	if !ok || !found || all.X >= official.X {
		t.Fatal("All must be the first marketplace tab")
	}
	app.capabilityUI.plugins.query = "missing"
	tt.Frame()
	if _, ok := tt.Find("Install fixture"); ok {
		t.Fatal("search did not filter catalog")
	}
	app.capabilityUI.plugins.query = "fixture"
	tt.Frame()
	if err := tt.Click("Community"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("Install fixture"); ok {
		t.Fatal("market tab did not filter catalog")
	}
	if err := tt.Click("All marketplaces"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("Install fixture"); !ok {
		t.Fatal("All did not restore catalog")
	}
	if err := tt.Click("Manage marketplace sources"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Refresh official"); err != nil {
		t.Fatal(err)
	}
	if len(service.commands) != 1 || service.commands[0].Kind != "plugins.source.refresh" {
		t.Fatalf("refresh did not dispatch typed source operation: %#v", service.commands)
	}
	if err := tt.Click("Edit official"); err != nil {
		t.Fatal(err)
	}
	app.capabilityUI.plugins.source.Ref = "pinned-commit"
	tt.Frame()
	if err := tt.Click("Save"); err != nil {
		t.Fatal(err)
	}
	saved := service.commands[len(service.commands)-1]
	if saved.Kind != "plugins.source.save" || saved.Plugin.Marketplace.ID != "official" || saved.Plugin.Marketplace.Ref != "pinned-commit" {
		t.Fatalf("source update lost identity or ref: %+v", saved)
	}
	if err := tt.Click("Remove community"); err != nil {
		t.Fatal(err)
	}
	if service.commands[len(service.commands)-1].Kind != "plugins.source.remove" {
		t.Fatal("source removal not dispatched")
	}
	if err := tt.Click("Close dialog"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Add marketplace source"); err != nil {
		t.Fatal(err)
	}
	if !app.capabilityUI.plugins.sourcesOpen || !app.capabilityUI.plugins.sourceOpen {
		t.Fatal("add source did not open editor dialog")
	}
	app.selectSettingsSection("Plugins")
	tt.Frame()
	if app.capabilityUI.plugins.sourcesOpen {
		t.Fatal("leaving marketplace retained source dialog")
	}
}

func TestPluginSettingsLargeCatalogBuildIsBounded(t *testing.T) {
	app, _ := pluginUIFixture(t)
	app.section = "Marketplace"
	app.state.Plugins.Catalog = nil
	for i := range 2300 {
		name := fmt.Sprintf("large-plugin-%04d", i)
		app.state.Plugins.Catalog = append(app.state.Plugins.Catalog, contracts.PluginCatalogItem{
			ID: name + "@official", Name: name, Marketplace: "official",
			Description: "A marketplace plugin with instructions, tools and a descriptive summary.",
		})
	}
	tt := pluginUITester(app)
	built := 0
	for _, text := range tt.Texts() {
		if strings.HasPrefix(text, "Install large-plugin-") {
			built++
		}
	}
	if built != 30 {
		t.Fatalf("marketplace built %d install controls for a 2300-entry catalog; expected one page (30)", built)
	}
	if err := tt.Click("Next page"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("Install large-plugin-0030"); !ok {
		t.Fatal("next page did not show its first plugin")
	}
	if _, ok := tt.Find("Install large-plugin-0000"); ok {
		t.Fatal("previous page remains in the native tree")
	}
	if err := tt.Click("Previous page"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("1–30 of 2300 plugins") {
		t.Fatal("previous page did not restore the first page")
	}
	app.capabilityUI.plugins.catalogPage = 76
	tt.Frame()
	if !tt.HasText("2281–2300 of 2300 plugins") {
		t.Fatal("last partial page omitted plugins")
	}
	if err := tt.Click("Next page"); err != nil {
		t.Fatal(err)
	}
	if app.capabilityUI.plugins.catalogPage != 76 {
		t.Fatal("disabled next-page action changed the page")
	}
	app.capabilityUI.plugins.query = "large-plugin-2299"
	tt.Frame()
	if _, ok := tt.Find("Install large-plugin-2299"); !ok || app.capabilityUI.plugins.catalogPage != 0 {
		t.Fatal("search did not find a plugin beyond the initial page or reset pagination")
	}
	if _, ok := tt.Find("Next page"); ok {
		t.Fatal("one result retained pagination controls")
	}
	app.capabilityUI.plugins.query = ""
	tt.Frame()
	if err := tt.Click("Next page"); err != nil {
		t.Fatal(err)
	}
	app.capabilityUI.plugins.marketplace = "community"
	tt.Frame()
	if app.capabilityUI.plugins.catalogPage != 0 || !tt.HasText("No plugins.") {
		t.Fatal("empty source did not reset pagination")
	}
	app.capabilityUI.plugins.marketplace = ""
	tt.Frame()
	if err := tt.Click("Next page"); err != nil {
		t.Fatal(err)
	}
	app.state.Plugins.Catalog = app.state.Plugins.Catalog[:1]
	tt.Frame()
	if _, ok := tt.Find("Install large-plugin-0000"); !ok || app.capabilityUI.plugins.catalogPage != 0 {
		t.Fatal("catalog shrink left the active page out of range")
	}
}

func TestPluginSettingsPaginationThemesAndNarrowWindows(t *testing.T) {
	for _, test := range []struct {
		name, language, next, previous string
		width                          int
		dark                           bool
	}{
		{"light", "en", "Next page", "Previous page", 1320, false},
		{"dark", "en", "Next page", "Previous page", 1320, true},
		{"narrow-chinese", "zh-CN", "下一页", "上一页", 700, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			app, service := pluginUIFixture(t)
			app.section = "Marketplace"
			app.state.Settings.General.Language = test.language
			app.palette = design.Palette("rcode-light", false)
			if test.dark {
				app.palette = design.Palette("rcode-dark", true)
			}
			for i := range 31 {
				name := fmt.Sprintf("page-plugin-%02d", i)
				app.state.Plugins.Catalog = append(app.state.Plugins.Catalog, contracts.PluginCatalogItem{
					ID: name + "@official", Name: name, Marketplace: "official",
				})
			}
			service.state = app.state.Plugins.Clone()
			tt := ui.NewTester(func(c *ui.Context) {
				localize.Set(c, test.language)
				c.SetTheme(app.palette.UI())
				app.settingsView(c)
			}, test.width, 1100)
			for _, label := range []string{test.next, test.previous} {
				r, ok := tt.Find(label)
				if !ok || r.X < 0 || r.X+r.W > float32(test.width) {
					t.Fatalf("pagination control %q overflows shared page: %+v", label, r)
				}
			}
			if err := tt.Click(test.next); err != nil {
				t.Fatal(err)
			}
			if !tt.HasText("page-plugin-30") {
				t.Fatal("last plugin missing on the second page")
			}
			label := "Install page-plugin-30"
			if test.language == "zh-CN" {
				label = "安装 page-plugin-30"
			}
			if err := tt.Click(label); err != nil {
				t.Fatal(err)
			}
			if len(service.commands) != 1 || service.commands[0].Plugin.Install.Name != "page-plugin-30" {
				t.Fatal("paged installation targeted the wrong plugin")
			}
		})
	}
}

func TestPluginSettingsRemovedSourceResetsToAll(t *testing.T) {
	app, _ := pluginUIFixture(t)
	app.section = "Marketplace"
	app.capabilityUI.plugins.marketplace = "community"
	tt := pluginUITester(app)
	app.state.Plugins.Marketplaces = app.state.Plugins.Marketplaces[:1]
	tt.Frame()
	if app.capabilityUI.plugins.marketplace != "" {
		t.Fatal("removed market left an invisible active filter")
	}
}

func TestPluginSettingsChineseAllTab(t *testing.T) {
	app, _ := pluginUIFixture(t)
	app.section = "Marketplace"
	app.state.Settings.General.Language = "zh-CN"
	tt := pluginUITester(app)
	tt.SetLocale(i18n.Native("zh-CN"))
	all, ok := tt.Find("所有")
	firstSource, found := tt.Find("Official")
	if !ok || !found || all.X >= firstSource.X {
		t.Fatal("Chinese marketplace must begin with the 所有 tab")
	}
}

func TestPluginSettingsRecognizesInstalledEntryWithDifferentRuntimeName(t *testing.T) {
	app, service := pluginUIFixture(t)
	app.section = "Marketplace"
	app.state.Plugins.Installed[0].Name = "runtime-namespace"
	tt := pluginUITester(app)
	_ = tt.Click("Install fixture")
	if len(service.commands) != 0 {
		t.Fatal("runtime name difference allowed a duplicate catalog installation")
	}
}

func TestPluginSettingsSecretDraftAndActions(t *testing.T) {
	app, service := pluginUIFixture(t)
	tt := pluginUITester(app)
	id := service.state.Installed[0].ID
	if err := tt.Click("Enable " + id); err != nil {
		t.Fatal(err)
	}
	if len(service.commands) != 1 || service.commands[0].Kind != "plugins.disable" {
		t.Fatal("toggle did not dispatch disable")
	}
	if err := tt.Click("Configure " + id); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Plugin token") {
		t.Fatal("configuration metadata not rendered")
	}
	if err := tt.Click("Plugin configuration Plugin token"); err != nil {
		t.Fatal(err)
	}
	tt.Type("draft-secret")
	if tt.HasText("draft-secret") {
		t.Fatal("secret was rendered as public text")
	}
	if err := tt.Click("Save plugin configuration"); err != nil {
		t.Fatal(err)
	}
	command := service.commands[len(service.commands)-1]
	if command.Kind != "plugins.configure" || command.Plugin.Configuration["token"] != "draft-secret" {
		t.Fatalf("typed configuration missing: %+v", command)
	}
	if app.capabilityUI.plugins.config != nil {
		t.Fatal("submitted secret draft retained")
	}
	if err := tt.Click("Configure " + id); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Remove saved value Plugin token"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Save plugin configuration"); err != nil {
		t.Fatal(err)
	}
	command = service.commands[len(service.commands)-1]
	if !slices.Equal(command.Plugin.RemoveConfigKeys, []string{"token"}) || len(command.Plugin.Configuration) != 0 {
		t.Fatalf("explicit configuration removal missing: %+v", command.Plugin)
	}
	if err := tt.Click("Configure " + id); err != nil {
		t.Fatal(err)
	}
	*app.capabilityUI.plugins.config["token"] = "cancelled-secret"
	app.selectSettingsSection("General")
	if app.capabilityUI.plugins.config != nil {
		t.Fatal("leaving plugin settings retained a secret")
	}
}

func TestPluginSettingsConfigurationTypesAndDefaultOwnership(t *testing.T) {
	app, service := pluginUIFixture(t)
	minimum, maximum := 1.0, 5.0
	fields := []contracts.PluginConfigField{
		{Key: "default_token", Title: "Default token", Type: "string", Sensitive: true, Configured: true, DefaultConfigured: true},
		{Key: "aliases", Title: "Aliases", Type: "string", Multiple: true},
		{Key: "retries", Title: "Retries", Type: "number", Min: &minimum, Max: &maximum},
		{Key: "enabled", Title: "Enabled", Type: "boolean", Configured: true, DefaultConfigured: true},
		{Key: "mode", Title: "Mode", Type: "string", Required: true, Options: []string{"fast", "slow"}},
	}
	app.state.Plugins.Installed[0].Config = fields
	service.state.Installed[0].Config = fields
	tt := pluginUITester(app)
	id := service.state.Installed[0].ID
	if err := tt.Click("Configure " + id); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("Remove saved value Default token"); ok {
		t.Fatal("a schema default was presented as a removable saved value")
	}
	for _, title := range []string{"Aliases (JSON string[])", "Retries (Number ≥ 1 ≤ 5)", "Enabled (true / false)"} {
		if !tt.HasText(title) {
			t.Fatalf("input format or numeric bounds missing: %q", title)
		}
	}
	if err := tt.Click("Save plugin configuration"); err != nil {
		t.Fatal(err)
	}
	first := service.commands[len(service.commands)-1].Plugin
	if len(first.Configuration) != 0 || len(first.RemoveConfigKeys) != 0 {
		t.Fatalf("unchanged defaults became saved overrides: %+v", first)
	}
	if err := tt.Click("Configure " + id); err != nil {
		t.Fatal(err)
	}
	for _, input := range []struct{ label, text string }{
		{"Aliases", `["alpha","beta"]`}, {"Retries", "3.5"},
	} {
		if err := tt.Click("Plugin configuration " + input.label); err != nil {
			t.Fatal(err)
		}
		tt.Type(input.text)
	}
	for _, input := range []struct{ label, option string }{{"Enabled", "false"}, {"Mode", "fast"}} {
		if err := tt.Click("Plugin configuration " + input.label); err != nil {
			t.Fatal(err)
		}
		if err := tt.Click("Plugin configuration " + input.label + " " + input.option); err != nil {
			t.Fatal(err)
		}
	}
	if err := tt.Click("Save plugin configuration"); err != nil {
		t.Fatal(err)
	}
	saved := service.commands[len(service.commands)-1].Plugin.Configuration
	if saved["enabled"] != "false" || saved["mode"] != "fast" || saved["retries"] != "3.5" || saved["aliases"] != `["alpha","beta"]` {
		t.Fatalf("typed configuration format changed: %+v", saved)
	}
	if _, exists := saved["default_token"]; exists {
		t.Fatal("public default metadata supplied a secret value")
	}
}

func TestPluginSettingsThemesNarrowAndOperationState(t *testing.T) {
	for _, test := range []struct {
		name  string
		width int
		dark  bool
	}{{"light", 1320, false}, {"dark", 1320, true}, {"narrow-dark", 700, true}} {
		t.Run(test.name, func(t *testing.T) {
			app, _ := pluginUIFixture(t)
			app.palette = design.Palette("rcode-light", false)
			if test.dark {
				app.palette = design.Palette("rcode-dark", true)
			}
			app.section = "Plugins"
			app.state.Plugins.Operations = []contracts.PluginOperation{
				{ID: "running", Kind: "plugins.source.refresh", TargetID: "official", State: "running"},
				{ID: "failed", Kind: "plugins.install", TargetID: "fixture", State: "failed", Error: "fixture install error"},
			}
			tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(app.palette.UI()); app.settingsView(c) }, test.width, 1100)
			for _, label := range []string{"Settings page viewport", "Settings page content", "Settings page heading: Plugins", "Browse plugin marketplace"} {
				if _, ok := tt.Find(label); !ok {
					t.Fatalf("missing shared page/control %q", label)
				}
			}
			if !tt.HasText("fixture install error") {
				t.Fatal("operation error omitted")
			}
			for _, page := range []struct{ section, control string }{
				{"Plugins", "Configure fixture@official"}, {"Marketplace", "Search plugins"},
			} {
				app.selectSettingsSection(page.section)
				tt.Frame()
				rect, ok := tt.Find(page.control)
				if !ok || rect.X < 0 || rect.X+rect.W > float32(test.width)+1 {
					t.Fatalf("%s control overflows the shared narrow page: %+v", page.section, rect)
				}
				if page.section == "Marketplace" {
					if !tt.HasText("Fixture dependency issue") {
						t.Fatal("catalog component issue omitted")
					}
					manage, ok := tt.Find("Manage marketplace sources")
					add, found := tt.Find("Add marketplace source")
					if !ok || !found || manage.X+manage.W > add.X || add.X+add.W > float32(test.width) {
						t.Fatalf("marketplace toolbar geometry invalid: %+v %+v", manage, add)
					}
					if err := tt.Click("Manage marketplace sources"); err != nil {
						t.Fatal(err)
					}
					rect, ok = tt.Find("Edit official")
					if !ok || rect.X < 0 || rect.X+rect.W > float32(test.width) {
						t.Fatalf("marketplace dialog overflows: %+v", rect)
					}
				}
				if os.Getenv("RCODE_CAPTURE_PLUGIN_SETTINGS") == "1" {
					folder := filepath.Join("..", "..", "output", "plugin-settings-split")
					if err := os.MkdirAll(folder, 0o755); err != nil {
						t.Fatal(err)
					}
					file, err := os.Create(filepath.Join(folder, test.name+"-"+page.section+".png"))
					if err != nil {
						t.Fatal(err)
					}
					err = png.Encode(file, tt.Image())
					file.Close()
					if err != nil {
						t.Fatal(err)
					}
				}
			}
		})
	}
}

func TestHookEditorPreservesCommandForm(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
	}{{"shell", nil}, {"exec-no-args", []string{}}, {"exec-with-args", []string{"a b", ""}}} {
		t.Run(test.name, func(t *testing.T) {
			app, _ := runtimeSettingsTestApp(t)
			hook := contracts.HookRule{ID: "form", Type: "command", Event: contracts.HookPreToolUse, Command: "program", Args: test.args}
			app.editHook(&hook)
			result, err := app.hookDraft()
			if err != nil || (result.Args == nil) != (test.args == nil) || !slices.Equal(result.Args, test.args) {
				t.Fatalf("command form changed: %#v -> %#v; %v", test.args, result.Args, err)
			}
		})
	}
}

func TestHookEditorFractionalTimeoutAndInvalidNumbers(t *testing.T) {
	app, _ := runtimeSettingsTestApp(t)
	hook := contracts.HookRule{ID: "fractional-timeout", Type: "command", Event: contracts.HookPreToolUse,
		Command: "echo fixture", TimeoutSeconds: 0.125}
	app.editHook(&hook)
	if app.capabilityUI.hookTimeout != "0.125" {
		t.Fatalf("fractional timeout was truncated in the editor: %q", app.capabilityUI.hookTimeout)
	}
	got, err := app.hookDraft()
	if err != nil || got.TimeoutSeconds != 0.125 {
		t.Fatalf("fractional timeout was lost: %g; %v", got.TimeoutSeconds, err)
	}
	for _, value := range []string{"NaN", "Inf", "-Inf", "1e309", "-0.1", "", "invalid"} {
		t.Run(value, func(t *testing.T) {
			app.capabilityUI.hookTimeout = value
			if _, err := app.hookDraft(); err == nil {
				t.Fatalf("invalid timeout %q accepted", value)
			}
		})
	}
}

func TestHookEditorTypedHandlersAndEventCatalog(t *testing.T) {
	app, _ := runtimeSettingsTestApp(t)
	for _, test := range []struct {
		name string
		hook contracts.HookRule
	}{
		{"http", contracts.HookRule{Type: "http", URL: "https://example.invalid/hook", Headers: map[string]string{"X-Fixture": "value"}, AllowedEnvVars: []string{"FIXTURE"}}},
		{"prompt", contracts.HookRule{Type: "prompt", Prompt: "Check $ARGUMENTS", Model: "haiku"}},
		{"agent", contracts.HookRule{Type: "agent", Prompt: "Inspect the result", Model: "sonnet"}},
		{"mcp-tool", contracts.HookRule{Type: "mcp_tool", MCPServer: "fixture", ToolName: "validate", ToolInput: json.RawMessage(`{"value":"fixture"}`)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.hook.ID, test.hook.Event, test.hook.Enabled = "typed", contracts.HookPreToolUse, true
			app.editHook(&test.hook)
			hook, err := app.hookDraft()
			if err != nil {
				t.Fatal(err)
			}
			if err := contracts.ValidateHookRules([]contracts.HookRule{hook}); err != nil {
				t.Fatal(err)
			}
			before, _ := json.Marshal(test.hook)
			after, _ := json.Marshal(hook)
			if string(before) != string(after) {
				t.Fatalf("handler changed while editing:\n%s\n%s", before, after)
			}
		})
	}
	app.editHook(nil)
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(app.palette.UI()); app.hookSettings(c) }, 1000, 1200)
	if err := tt.Click("Hook event"); err != nil {
		t.Fatal(err)
	}
	for _, event := range contracts.HookEvents() {
		if _, ok := tt.Find("Hook event " + string(event)); !ok {
			t.Fatalf("event %s missing from shared catalog", event)
		}
	}
}

func TestQuestionMultipleSelectionWaitsForSubmit(t *testing.T) {
	app, service := pluginUIFixture(t)
	question := &contracts.Question{ID: "multiple", Pages: []contracts.QuestionPage{{
		Question: "Choose features", Header: "Features", MultiSelect: true,
		Options: []contracts.QuestionOption{{Label: "Skills", Description: "Reusable instructions"}, {Label: "Hooks", Description: "Lifecycle callbacks"}},
	}}}
	app.state.Sessions = []contracts.Session{{ID: "question-session", Status: contracts.AwaitingInput}}
	app.sessionID = "question-session"
	app.state.Messages[app.sessionID] = []contracts.Message{{Blocks: []contracts.Block{{Kind: "question", Question: question}}}}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.questionComposer(c, app.sessionID, "test")
	}, 700, 800)
	if !tt.HasText("Reusable instructions") || !tt.HasText("Features") {
		t.Fatal("question metadata omitted")
	}
	for _, option := range question.Pages[0].Options {
		if err := tt.Click(option.Label); err != nil {
			t.Fatal(err)
		}
	}
	if len(service.commands) != 0 {
		t.Fatal("checkbox selection prematurely answered the question")
	}
	if err := tt.Click("Submit"); err != nil {
		t.Fatal(err)
	}
	if len(service.commands) != 1 || len(service.commands[0].QuestionAnswers) != 1 ||
		!slices.Equal(service.commands[0].QuestionAnswers[0].Options, []string{"Skills", "Hooks"}) {
		t.Fatalf("multi-select answer lost labels: %+v", service.commands)
	}
}
