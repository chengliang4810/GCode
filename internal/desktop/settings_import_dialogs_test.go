package desktop

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gcode/internal/contracts"
	localize "gcode/internal/i18n/view"
	"github.com/egoist/mygo/ui"
)

func TestSettingsImportRejectsInvalidObjects(t *testing.T) {
	tests := []struct {
		name string
		kind string
		json string
	}{
		{name: "null", kind: "command", json: "null"},
		{name: "array", kind: "command", json: "[]"},
		{name: "multiple", kind: "command", json: `{"name":"review","content":"Review"} {}`},
		{name: "unknown fields", kind: "command", json: `{"name":"review","content":"Review","project":"elsewhere"}`},
		{name: "empty command", kind: "command", json: `{"name":"review"}`},
		{name: "missing server command", kind: "mcp", json: `{"name":"tools","transport":"stdio"}`},
		{name: "unknown transport", kind: "mcp", json: `{"name":"tools","transport":"websocket","url":"https://example.test"}`},
		{name: "ambiguous server", kind: "mcp", json: `{"name":"tools","transport":"http","url":"https://example.test","command":"node"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := settingsImportState{kind: test.kind, id: "fresh-id", json: test.json}
			if _, err := s.command(); err == nil {
				t.Fatal("invalid import accepted")
			}
		})
	}
}

func TestSettingsImportAlwaysCreatesNewResource(t *testing.T) {
	app, core := runtimeSettingsTestApp(t)
	existing := contracts.UserCommand{ID: "existing-id", Name: "existing", Content: "Keep this template", Enabled: true}
	app.dispatch(contracts.Command{Kind: "resource.command.save", Resource: &contracts.ResourceInput{Command: &existing}})
	app.openSettingsImport("command")
	app.settingsImport.json = `{"id":"existing-id","name":"review","content":"Review $ARGUMENTS","enabled":true}`
	importedID := app.settingsImport.id
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Column(c).Fill().Children(func() { ui.Text(c, "Settings") })
		app.settingsImportDialog(c)
	}, 1000, 800)
	if err := tt.Click("Import"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if app.settingsImport.open {
		t.Fatalf("successful command import did not close: %+v", app.settingsImport)
	}
	snapshot, err := core.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := snapshot.UserResources.Commands; len(got) != 2 {
		t.Fatalf("import overwrote an existing command: %+v", got)
	}
	found := false
	for _, command := range snapshot.UserResources.Commands {
		if command.ID == importedID && command.Name == "review" {
			found = true
		}
		if command.ID == "existing-id" && command.Content != existing.Content {
			t.Fatal("import modified the existing template")
		}
	}
	if !found {
		t.Fatalf("import did not assign a fresh ID: %+v", snapshot.UserResources.Commands)
	}

	app.openSettingsImport("mcp")
	app.settingsImport.json = `{"id":"existing-mcp","name":"tools","transport":"stdio","command":"node","args":["server.js"],"env":{"TOKEN":"import-secret"},"enabled":true}`
	tt.Frame()
	if err := tt.Click("Import"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if app.settingsImport.open || app.settingsImport.json != "" {
		t.Fatalf("successful MCP import retained credentials: %+v", app.settingsImport)
	}
	snapshot, err = core.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := snapshot.UserResources.MCPServers; len(got) != 1 || got[0].ID == "existing-mcp" || len(got[0].EnvKeys) != 1 || got[0].EnvKeys[0] != "TOKEN" {
		t.Fatalf("MCP import not persisted: %+v", got)
	}
}

func TestSettingsImportFailureRetainsDraftAndIgnoresStaleCompletion(t *testing.T) {
	app, _ := runtimeSettingsTestApp(t)
	app.openSettingsImport("mcp")
	text := `{"name":"invalid name","transport":"stdio","command":"node","env":{"TOKEN":"retry-secret"}}`
	app.settingsImport.json = text
	app.saveSettingsImport()
	if !app.settingsImport.open || app.settingsImport.json != text || app.settingsImport.errorText == "" || app.settingsImport.pendingID != "" {
		t.Fatalf("failed import lost its draft: %+v", app.settingsImport)
	}
	oldID := app.settingsImport.id
	app.openSettingsImport("mcp")
	newID := app.settingsImport.id
	app.settingsImportCommandFinished(contracts.Command{
		Kind:     "resource.mcp.save",
		Resource: &contracts.ResourceInput{MCP: &contracts.MCPServerInput{ID: oldID}},
	}, errors.New("old failure"))
	if app.settingsImport.id != newID || app.settingsImport.errorText != "" {
		t.Fatal("stale import completion modified the new draft")
	}
}

func TestSettingsImportDialogValidatesBeforeDispatchAndCancels(t *testing.T) {
	app, _ := runtimeSettingsTestApp(t)
	app.openSettingsImport("command")
	app.settingsImport.json = "null"
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Column(c).Fill().Children(func() { ui.Text(c, "Settings") })
		app.settingsImportDialog(c)
	}, 1000, 800)
	if err := tt.Click(app.t("Import")); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !app.settingsImport.open || !strings.Contains(app.settingsImport.errorText, "JSON object") {
		t.Fatalf("invalid JSON was not retained in the modal: %+v", app.settingsImport)
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if app.settingsImport.open || app.settingsImport.json != "" || tt.HasText(app.t("Configuration JSON")) {
		t.Fatal("cancel did not dismiss and clear the import")
	}
}

func TestSettingsImportToolbarOpensModalInBothLanguages(t *testing.T) {
	for _, language := range []string{"en", "zh-CN"} {
		for _, page := range []struct{ section, kind, title string }{
			{section: "MCP servers", kind: "mcp", title: "Import MCP server"},
			{section: "Commands", kind: "command", title: "Import command"},
		} {
			t.Run(page.kind+"-"+language, func(t *testing.T) {
				app, _ := runtimeSettingsTestApp(t)
				app.state.Settings.General.Language = language
				app.route, app.section = "settings", page.section
				tt := ui.NewTester(func(c *ui.Context) {
					localize.Set(c, language)
					c.SetTheme(app.palette.UI())
					ui.Row(c).Fill().Children(func() { app.settingsView(c) })
				}, 1100, 850)
				if err := tt.Click(app.t("Import")); err != nil {
					t.Fatal(err)
				}
				tt.Frame()
				if !app.settingsImport.open || app.settingsImport.kind != page.kind || !tt.HasText(app.t(page.title)) {
					t.Fatalf("toolbar did not open the matching modal: %+v", app.settingsImport)
				}
				for _, label := range []string{"Configuration JSON", "Cancel", "Close dialog"} {
					bounds, ok := tt.Find(app.t(label))
					if !ok || bounds.X < 0 || bounds.Y < 0 || bounds.X+bounds.W > 1100 || bounds.Y+bounds.H > 850 {
						t.Fatalf("modal control is absent or clipped: %s %+v", label, bounds)
					}
				}
				tt.Key(0, ui.KeyEscape)
				tt.Frame()
				if app.settingsImport.open || app.settingsImport.json != "" {
					t.Fatal("Escape did not dismiss and clear the import draft")
				}
			})
		}
	}
}
