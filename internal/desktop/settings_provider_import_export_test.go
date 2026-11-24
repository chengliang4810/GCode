package desktop

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/model"
	"github.com/egoist/mygo/ui"
)

func providerImportExportTester(app *App) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Column(c).Fill().Padding(16).Gap(12).Children(func() { app.customProviderSettings(c) })
	}, 1100, 900)
}

func TestProviderImportOpensLeftOfAddProvider(t *testing.T) {
	app, _ := runtimeSettingsTestApp(t)
	tt := providerImportExportTester(app)
	importRect, ok := tt.Find(app.t("Import provider"))
	if !ok || importRect.W <= 0 {
		t.Fatalf("missing import-provider action: %+v\n%v", importRect, tt.Texts())
	}
	addRect, ok := tt.Find(app.t("Add provider"))
	if !ok || addRect.W <= 0 {
		t.Fatalf("missing add-provider action: %+v\n%v", addRect, tt.Texts())
	}
	if importRect.X >= addRect.X {
		t.Fatalf("import-provider must sit left of add-provider: import=%+v add=%+v", importRect, addRect)
	}
	if importRect.Y != addRect.Y {
		t.Fatalf("import and add actions must share the header row: import=%+v add=%+v", importRect, addRect)
	}
	if err := tt.Click(app.t("Import provider")); err != nil {
		t.Fatal(err)
	}
	if !app.providerImport.open {
		t.Fatal("import-provider did not open the import dialog")
	}
	if !tt.HasText(app.t("Provider JSON")) {
		t.Fatalf("import dialog did not render its JSON field: %v", tt.Texts())
	}
}

func TestProviderImportPersistsPastedJSONAsNewProvider(t *testing.T) {
	app, core := runtimeSettingsTestApp(t)
	tt := providerImportExportTester(app)
	if err := tt.Click(app.t("Import provider")); err != nil {
		t.Fatal(err)
	}
	if app.providerImport.text != providerJSONSample {
		t.Fatal("import dialog did not start from the documented sample")
	}
	app.providerImport.text = `{"id":"pasted-identity","name":" Imported provider ","protocol":"openai-chat",
		"baseUrl":"https://imported.example.com/v1","apiKey":" imported-secret ",
		"models":[{"id":"imported-model"}]}`
	tt.Frame()
	if err := tt.Click(app.t("Import")); err != nil {
		t.Fatal(err)
	}
	state, err := core.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(state.CustomProviders) != 1 {
		t.Fatalf("import did not save exactly one provider: %+v (%s)", state.CustomProviders, app.errorText)
	}
	saved := state.CustomProviders[0].Provider
	if saved.ID == "pasted-identity" || !strings.HasPrefix(saved.ID, "provider-") {
		t.Fatalf("import must mint its own provider ID: %q", saved.ID)
	}
	if saved.Name != "Imported provider" || saved.BaseURL != "https://imported.example.com/v1" ||
		saved.Protocol != model.Chat || len(saved.Models) != 1 || saved.Models[0].ID != "imported-model" {
		t.Fatalf("import did not persist the pasted configuration: %+v", saved)
	}
	if saved.APIKey != "" {
		t.Fatal("import wrote the plaintext key into the snapshot")
	}
	key, err := core.RevealProviderKey(context.Background(), saved.ID)
	if err != nil || key != "imported-secret" {
		t.Fatalf("imported key was not stored: %q, %v", key, err)
	}
	if app.providerImport.open {
		t.Fatal("successful import left the dialog open")
	}
	if app.modelsUI.selected != saved.ID {
		t.Fatalf("import did not select the new provider: %q", app.modelsUI.selected)
	}
}

func TestProviderImportRejectsInvalidJSON(t *testing.T) {
	app, core := runtimeSettingsTestApp(t)
	tt := providerImportExportTester(app)
	if err := tt.Click(app.t("Import provider")); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{
		`{"protocol":"openai-chat","baseUrl":"https://example.com/v1"}`,
		`{"name":"Unknown field","protocol":"openai-chat","baseUrl":"https://example.com/v1","unexpected":true}`,
		`{"name":"Bad protocol","protocol":"openai","baseUrl":"https://example.com/v1"}`,
		`[]`,
	} {
		app.providerImport.text = text
		tt.Frame()
		if err := tt.Click(app.t("Import")); err != nil {
			t.Fatal(err)
		}
		if !app.providerImport.open || app.providerImport.errorText == "" {
			t.Fatalf("invalid import %q was accepted: open=%t error=%q", text, app.providerImport.open, app.providerImport.errorText)
		}
	}
	state, err := core.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(state.CustomProviders) != 0 {
		t.Fatal("a rejected import reached the core")
	}
}

func TestProviderImportReadsChoosenJSONFile(t *testing.T) {
	app, _ := runtimeSettingsTestApp(t)
	path := filepath.Join(t.TempDir(), "provider.json")
	document := `{"name":"From file","protocol":"openai-chat","baseUrl":"https://file.example.com/v1","models":[{"id":"file-model"}]}`
	if err := os.WriteFile(path, []byte(document), 0600); err != nil {
		t.Fatal(err)
	}
	app.chooseProviderImportPath = func() (string, error) { return path, nil }
	tt := providerImportExportTester(app)
	if err := tt.Click(app.t("Import provider")); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(app.t("Choose file")); err != nil {
		t.Fatal(err)
	}
	if app.providerImport.text != document {
		t.Fatalf("choosen file did not reach the JSON field: %q", app.providerImport.text)
	}
	if app.providerImport.errorText != "" || app.providerImport.loading {
		t.Fatalf("reading a small file left import state dirty: %+v", app.providerImport)
	}
	if err := tt.Click(app.t("Import")); err != nil {
		t.Fatal(err)
	}
	if app.providerImport.open {
		t.Fatal("file import did not close the dialog after saving")
	}
	if app.providerImport.text != "" {
		t.Fatal("closing the dialog retained the imported credentials")
	}
}

func TestProviderExportListsExportBelowRemoveAndSavesPlainTextJSON(t *testing.T) {
	app, _ := runtimeSettingsTestApp(t)
	provider := model.Provider{ID: "exported", Name: "Export Me", Protocol: model.Chat,
		BaseURL: "https://export.example.com/v1", APIKey: "export-secret",
		Models: []model.Model{{ID: "export-model"}}}
	app.dispatch(contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider}})
	app.modelsUI.selected = "exported"
	tt := providerImportExportTester(app)
	if err := tt.Click(app.t("Provider actions")); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	removeAt, exportAt := slices.Index(tt.Menu(), app.t("Remove provider")), slices.Index(tt.Menu(), app.t("Export provider"))
	if removeAt < 0 || exportAt < 0 {
		t.Fatalf("provider menu does not list both removal and export: %v", tt.Menu())
	}
	if exportAt <= removeAt {
		t.Fatalf("export must be listed below removal: %v", tt.Menu())
	}
	if err := tt.ChooseMenuItem(app.t("Export provider")); err != nil {
		t.Fatal(err)
	}
	if !app.providerExport.open || app.providerExport.providerID != "exported" {
		t.Fatalf("export did not open for the provider: %+v", app.providerExport)
	}
	if app.providerExport.loading || app.providerExport.errorText != "" {
		t.Fatalf("export did not finish reading the key: %+v", app.providerExport)
	}
	if !strings.Contains(app.providerExport.json, "Export Me") ||
		!strings.Contains(app.providerExport.json, "export-secret") ||
		!strings.HasSuffix(app.providerExport.json, "\n") {
		t.Fatalf("exported JSON is not the plaintext provider document: %q", app.providerExport.json)
	}
	if !tt.HasText(app.t("This JSON contains the API key in plain text. Keep the file private.")) {
		t.Fatal("export dialog did not warn about the plaintext key")
	}
	target := filepath.Join(t.TempDir(), "export.json")
	app.chooseProviderExportPath = func() (string, error) { return target, nil }
	tt.Frame()
	if err := tt.Click(app.t("Save as file")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != app.providerExport.json {
		t.Fatalf("saved file does not match the exported JSON: %q", string(data))
	}
	// The saved slip is what the import side accepts, so a round trip restores
	// the same connection settings under a fresh ID.
	if err := tt.Click(app.t("Close")); err != nil {
		t.Fatal(err)
	}
	if app.providerExport.open {
		t.Fatal("closing the export dialog left it open")
	}
	if err := tt.Click(app.t("Import provider")); err != nil {
		t.Fatal(err)
	}
	app.providerImport.text = string(data)
	tt.Frame()
	if err := tt.Click(app.t("Import")); err != nil {
		t.Fatal(err)
	}
	if app.providerImport.open || len(app.state.CustomProviders) != 2 {
		t.Fatalf("round-tripped export did not import: open=%t providers=%d error=%q",
			app.providerImport.open, len(app.state.CustomProviders), app.providerImport.errorText)
	}
	imported := app.state.CustomProviders[1].Provider
	if imported.Name != "Export Me" || imported.BaseURL != provider.BaseURL || imported.ID == provider.ID {
		t.Fatalf("round trip changed the configuration: %+v", imported)
	}
}

func TestProviderImportExportDialogsFitEveryThemeAndWidth(t *testing.T) {
	app, _ := runtimeSettingsTestApp(t)
	provider := model.Provider{ID: "narrow", Name: "Narrow provider", Protocol: model.Chat,
		BaseURL: "https://narrow.example.com/v1", APIKey: "narrow-secret",
		Models: []model.Model{{ID: "narrow-model"}}}
	app.dispatch(contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider}})
	app.modelsUI.selected = "narrow"
	for _, dark := range []bool{false, true} {
		for _, width := range []int{520, 1100} {
			theme := "rcode-light"
			if dark {
				theme = "rcode-dark"
			}
			app.palette = design.Palette(theme, dark)
			app.providerImport, app.providerExport = providerImportState{}, providerExportState{}
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(app.palette.UI())
				ui.Column(c).Fill().Padding(16).Gap(12).Children(func() { app.customProviderSettings(c) })
			}, width, 800)
			if err := tt.Click(app.t("Import provider")); err != nil {
				t.Fatal(err)
			}
			assertDialogField := func(key string, width int, action string) {
				t.Helper()
				rect, ok := tt.Find(app.t("Provider JSON"))
				if !ok || rect.W <= 0 {
					t.Fatalf("%s dialog has no JSON field at %d px: %v", key, width, tt.Texts())
				}
				if rect.X < 0 || rect.X+rect.W > float32(width)+1 {
					t.Fatalf("%s dialog overflows a %d px window: %+v", key, width, rect)
				}
				actionRect, ok := tt.Find(app.t(action))
				if !ok || actionRect.W <= 0 {
					t.Fatalf("%s dialog lost %q at %d px: %v", key, action, width, tt.Texts())
				}
				if actionRect.X+actionRect.W > float32(width)+1 {
					t.Fatalf("%s dialog %q overflows a %d px window: %+v", key, action, width, actionRect)
				}
			}
			assertDialogField(theme+" import", width, "Choose file")
			if err := tt.Click(app.t("Cancel")); err != nil {
				t.Fatal(err)
			}
			if err := tt.Click(app.t("Provider actions")); err != nil {
				t.Fatal(err)
			}
			if err := tt.ChooseMenuItem(app.t("Export provider")); err != nil {
				t.Fatal(err)
			}
			assertDialogField(theme+" export", width, "Save as file")
			if err := tt.Click(app.t("Close")); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestProviderExportFileName(t *testing.T) {
	for _, test := range []struct{ name, want string }{
		{"Example provider", "example-provider.json"},
		{"  My-Provider_2  ", "my-provider_2.json"},
		{"供应商 / 一", "provider.json"},
		{"", "provider.json"},
	} {
		if got := providerExportFileName(test.name); got != test.want {
			t.Fatalf("providerExportFileName(%q) = %q, want %q", test.name, got, test.want)
		}
	}
}
