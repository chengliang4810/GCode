package desktop

import (
	"encoding/json"
	"fmt"
	"gcode/internal/contracts"
	"gcode/internal/mock"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestCaptureModelPickerAudit(t *testing.T) {
	if os.Getenv("RCODE_MODEL_AUDIT_DIR") == "" {
		t.Skip("explicit audit capture directory required")
	}
	for _, appearance := range []string{"light", "dark"} {
		a := modelSurfaceTestApp(t, appearance)
		a.effortMenu, a.modelMenu = false, true
		tt := ui.NewTester(a.View, 1320, 880)
		writeModelSurfaceEvidence(t, "model-list-"+appearance, tt, map[string]any{"scope": "implementation layout"})
	}
}

func openTestModelList(tt *ui.Tester, label string) error {
	if err := tt.Click(label); err != nil {
		return err
	}
	// Exercise the preserved explicit search path even for small catalogs.
	tt.Key(ui.Cmd, ui.KeyF)
	return nil
}

func TestModelPickerUnifiedNavigation(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 900, 600)
	if err := openTestModelList(tt, "Select model"); err != nil {
		t.Fatal(err)
	}
	if !tt.Focused("Search models") {
		t.Fatal("explicit search did not focus")
	}
	if _, ok := tt.Find("Change model"); ok {
		t.Fatal("removed intermediate page remains")
	}
	if err := tt.Click("Codex"); err != nil {
		t.Fatal(err)
	}
	tt.Type("GPT-5.3")
	tt.Key(0, ui.KeyEnter)
	if a.modelID != "gpt-5.3-codex" || a.modelMenu {
		t.Fatal("selection did not commit and dismiss")
	}
	if err := tt.Click("Select model"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyEscape)
	if a.modelMenu {
		t.Fatal("Escape did not close the menu directly")
	}
	payload, _ := json.Marshal(a.state.Settings.General)
	if strings.Contains(string(payload), "compactModelPicker") {
		t.Fatal("removed setting is persisted")
	}
}

func TestModelPickerProviderFiltersDoNotChangeSelection(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 1320, 880)
	before := a.modelID
	if err := openTestModelList(tt, "Select model"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Codex"); err != nil {
		t.Fatal(err)
	}
	if a.modelID != before {
		t.Fatal("filter changed committed model")
	}
	for _, row := range a.modelRows() {
		if row.provider.ID != "openai" {
			t.Fatal("provider filter mixed scopes")
		}
	}
	if err := tt.Click("GPT-5.4"); err != nil {
		t.Fatal(err)
	}
	if a.modelID != "gpt-5.4" || a.modelMenu {
		t.Fatal("model click failed to commit and dismiss")
	}
	if err := tt.Click("Select model"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Model management"); err != nil {
		t.Fatal(err)
	}
	if a.route != "settings" || a.section != "Providers" || a.modelID != "gpt-5.4" {
		t.Fatal("model settings entry changed the selected model or failed to navigate")
	}

}

func TestCaptureModelCatalogAudit(t *testing.T) {
	dir := os.Getenv("RCODE_MODEL_AUDIT_DIR")
	if dir == "" {
		t.Skip("explicit audit capture directory required")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"light", "dark"} {
		for _, status := range []contracts.ModelCatalogStatus{contracts.CatalogLoading, contracts.CatalogError, contracts.CatalogEmpty, contracts.CatalogNoAgents} {
			a := testApp(t, "default")
			a.state.Settings.Appearance.Mode = mode
			a.state.Settings.Appearance.ReduceMotion = true
			a.state.Settings.Appearance.ThemeID = "rcode-" + mode
			a.state.ModelCatalog = contracts.ModelCatalogState{State: status, Error: "The model catalog could not be loaded."}
			if status == contracts.CatalogEmpty || status == contracts.CatalogNoAgents {
				a.state.Models = nil
				a.modelID = ""
			}
			tt := ui.NewTester(a.View, 1320, 880)
			if err := openTestModelList(tt, "Select model"); err != nil {
				t.Fatal(err)
			}
			f, err := os.Create(filepath.Join(dir, "catalog-"+string(status)+"-"+mode+".png"))
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(f, tt.Image()); err != nil {
				_ = f.Close()
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestModelPickerBoundsAtWindowEdges(t *testing.T) {
	for _, size := range [][2]int{{900, 600}, {1320, 880}, {1600, 1000}} {
		a := testApp(t, "default")
		a.selectSession("chat-0")
		a.files, a.right = true, true
		tt := ui.NewTester(a.View, size[0], size[1])
		if err := openTestModelList(tt, "Select model"); err != nil {
			t.Fatal(err)
		}
		tt.Advance(time.Second)
		r, ok := tt.Find("Model picker")
		anchor, _ := tt.Find("Select model")
		maxWidth := min(modelPickerMaxWidth, float32(size[0]-16))
		if !ok || r.X < 0 || r.Y < 0 || r.X+r.W > float32(size[0]) || r.Y+r.H > anchor.Y || r.W <= 0 || r.W > maxWidth+.5 {
			t.Fatalf("popup escaped %v: %+v anchor=%+v", size, r, anchor)
		}
	}
}

func TestModelPickerVirtualizesLargeCatalogAndNoResults(t *testing.T) {
	a := testApp(t, "default")
	a.state.Models = nil
	for i := 0; i < 7000; i++ {
		a.state.Models = append(a.state.Models, contracts.Model{ID: fmt.Sprintf("model-%d", i), ProviderID: "claude", Label: fmt.Sprintf("Model %04d with a long descriptive name that must stay on a single row", i)})
	}
	a.modelID = "model-0"
	a = modelSnapshotApp(t, a.state)
	tt := ui.NewTester(a.View, 900, 600)
	if err := openTestModelList(tt, "Select model"); err != nil {
		t.Fatal(err)
	}
	first, last := a.modelList.Visible()
	if last-first > 12 {
		t.Fatalf("large model catalog was not virtualized: %d..%d", first, last)
	}
	tt.Type("missing-model")
	if !tt.HasText("No models found") || a.modelCursor != -1 {
		t.Fatal("search empty state missing")
	}
	tt.Key(0, ui.KeyEnter)
	if a.modelID != "model-0" {
		t.Fatal("empty search selected a model")
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("Model 6999")
	tt.Key(0, ui.KeyEnter)
	if a.modelID != "model-6999" {
		t.Fatalf("large catalog result did not activate: %s", a.modelID)
	}
}

func modelSnapshotApp(t *testing.T, snapshot contracts.Snapshot) *App {
	t.Helper()
	data, err := json.Marshal(struct {
		Snapshot contracts.Snapshot `json:"snapshot"`
		Next     uint64             `json:"next"`
	}{snapshot, 1000})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	backend, err := mock.New(mock.Options{StatePath: path, Instant: true})
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(backend)
	if err != nil {
		_ = backend.Close()
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	return a
}

func TestInitialModelResolutionStaysWithinPreferredProvider(t *testing.T) {
	for _, memory := range []string{"", "removed-model"} {
		a := testApp(t, "default")
		a.state.Settings.ModelPicker.ProviderID = "openai"
		a.state.Settings.ModelPicker.Selections = map[string]contracts.ModelSelection{"openai": {ModelID: memory}}
		a = modelSnapshotApp(t, a.state)
		if a.selectedModel().ProviderID != "openai" {
			t.Fatalf("preferred provider silently changed: %s", a.modelID)
		}
	}
}

func TestUnresolvedModelCannotSilentlySendDefaultAndRetryResolvesIt(t *testing.T) {
	a := testApp(t, "default")
	a.state.Models = nil
	a.state.ModelCatalog = contracts.ModelCatalogState{State: contracts.CatalogError, Error: "Model catalog unavailable"}
	a.state.Settings.ModelPicker.ProviderID = "openai"
	a = modelSnapshotApp(t, a.state)
	tt := ui.NewTester(a.View, 1320, 880)
	if err := tt.Click("Message"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Keep this task for Codex")
	count := len(a.state.Sessions)
	tt.Key(0, ui.KeyEnter)
	if len(a.state.Sessions) != count || a.draft != "Keep this task for Codex" || a.modelID != "" {
		t.Fatal("unresolved catalog sent a default model or discarded the draft")
	}
	a.dispatch(contracts.Command{Kind: "catalog.retry"})
	if a.selectedModel().ProviderID != "openai" {
		t.Fatalf("retry did not resolve the preferred provider: %s", a.modelID)
	}
}

func TestStandardPickerReopensOnCurrentLockedProvider(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	a.modelProvider = "claude"
	for i := range a.state.Sessions {
		if a.state.Sessions[i].ID == "chat-1" {
			a.state.Sessions[i].ModelID = "gpt-5.4"
			a.state.Sessions[i].Status = contracts.Working
		}
	}
	a.selectSession("chat-1")
	tt := ui.NewTester(a.View, 1320, 880)
	if err := openTestModelList(tt, "Select model"); err != nil {
		t.Fatal(err)
	}
	rows := a.modelRows()
	if len(rows) == 0 {
		t.Fatal("old provider filter hid the active session's models")
	}
	for _, row := range rows {
		if row.model.ProviderID != "openai" {
			t.Fatal("working session exposed a foreign provider")
		}
	}
}

func TestAllDisabledProvidersHaveNoAgentsButKeepExistingChatModel(t *testing.T) {
	a := testApp(t, "default")
	for i := range a.state.Providers {
		a.state.Providers[i].Enabled = false
	}
	for i := range a.state.Integrations.Providers {
		a.state.Integrations.Providers[i].Enabled = false
	}
	a = modelSnapshotApp(t, a.state)
	if !a.modelNoAgents() || a.modelLabel() != "Model management" {
		t.Fatal("all-disabled catalog remained a perpetual loading list")
	}
	a.selectSession("chat-0")
	if a.modelLabel() != "Opus 4.6" || len(a.modelRows()) == 0 {
		t.Fatal("existing chat lost its committed provider/model")
	}
}

func TestIndependentReasoningPickerAndModelSelection(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 1320, 880)
	if err := tt.Click("Select reasoning effort"); err != nil {
		t.Fatal(err)
	}
	if a.modelMenu || !a.effortMenu {
		t.Fatal("reasoning control opened the model list")
	}
	if err := tt.Click("Reasoning effort"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyEnd)
	if a.reasoning != "Max" {
		t.Fatal("slider did not update effort")
	}
	if err := openTestModelList(tt, "Select model"); err != nil {
		t.Fatal(err)
	}
	if a.effortMenu || !a.modelMenu {
		t.Fatal("model list did not replace reasoning popup")
	}
	if _, ok := tt.Find("Reasoning effort"); ok {
		t.Fatal("model list still contains reasoning slider")
	}
	if err := tt.Click("Codex"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("GPT-5.4"); err != nil {
		t.Fatal(err)
	}
	if a.modelID != "gpt-5.4" || a.reasoning == "Max" || a.modelMenu {
		t.Fatal("model choice retained incompatible effort or open list")
	}
}

func TestModelSearchMatchesProviderName(t *testing.T) {
	a := testApp(t, "default")
	a.modelProvider, a.modelQuery = "openai", "codex"
	if len(a.modelRows()) == 0 {
		t.Fatal("search did not match provider name")
	}
	for _, row := range a.modelRows() {
		if row.provider.ID != "openai" {
			t.Fatal("search mixed provider scopes")
		}
	}
}

func TestModelPickerRemovedFavoriteAndNumberShortcuts(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 1320, 880)
	if err := openTestModelList(tt, "Select model"); err != nil {
		t.Fatal(err)
	}
	before := a.modelID
	settingsBefore, _ := json.Marshal(a.state.Settings.ModelPicker)
	tt.Key(0, ui.KeyDown)
	tt.Key(ui.Cmd|ui.Shift, ui.KeyF)
	for key := ui.Key1; key <= ui.Key9; key++ {
		tt.Key(ui.Cmd, key)
	}
	settingsAfter, _ := json.Marshal(a.state.Settings.ModelPicker)
	if a.modelID != before || string(settingsBefore) != string(settingsAfter) {
		t.Fatal("removed picker shortcuts changed model configuration")
	}
	if tt.HasText("Favorites") || strings.Contains(string(settingsAfter), "favorites") {
		t.Fatal("model favorites remain in the UI or settings")
	}
}

func TestModelProviderStaysLockedDuringActiveTurn(t *testing.T) {
	for _, status := range []contracts.SessionStatus{contracts.Working, contracts.AwaitingInput} {
		a := testApp(t, "default")
		a.selectSession("chat-0")
		for i := range a.state.Sessions {
			if a.state.Sessions[i].ID == a.sessionID {
				a.state.Sessions[i].Status = status
			}
		}
		provider := a.selectedModel().ProviderID
		for _, row := range a.modelRows() {
			if row.model.ProviderID != provider {
				t.Fatal("active run offered a foreign provider")
			}
		}
		a.centralControls().activateModelRow([]modelRow{{model: contracts.Model{ID: "gpt-5.4", ProviderID: "openai"}, provider: contracts.Provider{ID: "openai"}}}, 0)
		if a.selectedModel().ProviderID != provider {
			t.Fatal("provider changed while a turn was active")
		}
	}
}

func TestModelPickerPreservesShellFillAndProviderChoice(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 1320, 880)
	before := tt.Image().RGBAAt(800, 200)
	if err := openTestModelList(tt, "Select model"); err != nil {
		t.Fatal(err)
	}
	if got := tt.Image().RGBAAt(800, 200); got != before {
		t.Fatalf("model list changed the shell fill: %v -> %v", before, got)
	}
	if err := tt.Click("Codex"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("GPT-5.4"); err != nil {
		t.Fatal(err)
	}
	fresh, err := New(a.service)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fresh.Close)
	if fresh.modelID != "gpt-5.4" || fresh.reasoning != a.reasoning {
		t.Fatalf("new view lost provider memory: %s / %s", fresh.modelID, fresh.reasoning)
	}
}

func TestModelCatalogLoadingErrorRetryAndNoAgents(t *testing.T) {
	a := testApp(t, "default")
	a.state.Settings.Appearance.ReduceMotion = true
	a.saveSettings()
	a.state.ModelCatalog = contracts.ModelCatalogState{State: contracts.CatalogLoading}
	tt := ui.NewTester(a.View, 900, 600)
	if err := openTestModelList(tt, "Select model"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Loading models") || tt.HasText("Search models") {
		t.Fatal("loading catalog did not replace the unstable list chrome")
	}
	before := tt.Image().RGBAAt(500, 100)
	tt.Advance(3 * time.Second)
	if before != tt.Image().RGBAAt(500, 100) {
		t.Fatal("reduced-motion catalog changed its shell fill")
	}
	a.state.ModelCatalog = contracts.ModelCatalogState{State: contracts.CatalogError, Error: "The model catalog could not be loaded."}
	tt.Frame()
	if !tt.HasText(a.state.ModelCatalog.Error) {
		t.Fatal("catalog error missing")
	}
	if err := tt.Click("Retry"); err != nil {
		t.Fatal(err)
	}
	if a.state.ModelCatalog.State != contracts.CatalogReady || !tt.HasText("Sonnet 4.6") {
		t.Fatalf("retry did not restore the catalog: %v error %q open %v texts %v", a.state.ModelCatalog, a.errorText, a.modelMenu, tt.Texts())
	}
	a.state.ModelCatalog = contracts.ModelCatalogState{State: contracts.CatalogNoAgents}
	for i := range a.state.Integrations.Providers {
		a.state.Integrations.Providers[i].Enabled = false
	}
	tt.Frame()
	if !tt.HasText("No models configured") || !tt.HasText("Configure a provider") {
		t.Fatal("no agents state missing")
	}
	a.modelMenu = false
	tt.Frame()
	if err := tt.Click("Message"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Keep my draft")
	count := len(a.state.Sessions)
	tt.Key(0, ui.KeyEnter)
	if len(a.state.Sessions) != count || a.draft != "Keep my draft" {
		t.Fatal("no-agents send lost the draft or created a session")
	}
}
