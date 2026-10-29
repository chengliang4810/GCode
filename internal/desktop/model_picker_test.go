package desktop

import (
	"encoding/json"
	"fmt"
	"gcode/internal/contracts"
	"gcode/internal/mock"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestCaptureModelPickerAudit(t *testing.T) {
	dir := os.Getenv("ZERON_MODEL_AUDIT_DIR")
	if dir == "" {
		t.Skip("explicit audit capture directory required")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"light", "dark"} {
		a := testApp(t, "default")
		a.state.Settings.Appearance.Mode = mode
		a.state.Settings.Appearance.ThemeID = "zeron-" + mode
		tt := ui.NewTester(a.View, 1320, 880)
		save := func(name string) {
			f, err := os.Create(filepath.Join(dir, name+"-"+mode+".png"))
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
			if mode == "light" && (name == "compact" || name == "list") {
				r, ok := tt.Find("Model picker")
				if !ok {
					t.Fatal("model popup missing from capture")
				}
				bounds := image.Rect(int(r.X)-20, int(r.Y)-20, int(r.X+r.W)+20, int(r.Y+r.H)+20).Intersect(tt.Image().Bounds())
				focus, err := os.Create(filepath.Join(dir, "focus-"+name+"-"+mode+".png"))
				if err != nil {
					t.Fatal(err)
				}
				if err := png.Encode(focus, tt.Image().SubImage(bounds)); err != nil {
					_ = focus.Close()
					t.Fatal(err)
				}
				if err := focus.Close(); err != nil {
					t.Fatal(err)
				}
			}
		}
		if err := tt.Click("Select model"); err != nil {
			t.Fatal(err)
		}
		save("compact")
		if err := tt.Click("Change model"); err != nil {
			t.Fatal(err)
		}
		save("list")
		a.modelMenu = false
		a.state.Settings.General.CompactModelPicker = false
		tt.Frame()
		if err := tt.Click("Select model"); err != nil {
			t.Fatal(err)
		}
		save("full")
	}
}

func TestModelPickerSearchKeyboardAndEscape(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 900, 600)
	if err := tt.Click("Select model"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Change model"); err != nil {
		t.Fatal(err)
	}
	tt.Type("GPT-5.3")
	if a.modelQuery != "GPT-5.3" || len(a.modelRows()) != 1 {
		t.Fatalf("search did not filter models: %q, %d", a.modelQuery, len(a.modelRows()))
	}
	tt.Key(0, ui.KeyEnter)
	if a.modelID != "gpt-5.3-codex" || !a.modelMenu || a.modelListPage {
		t.Fatalf("Enter did not select and return to options: %q, cursor %d, open %v list %v rows %+v", a.modelID, a.modelCursor, a.modelMenu, a.modelListPage, a.modelRows())
	}
	if err := tt.Click("Change model"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyEscape)
	if !a.modelMenu || a.modelListPage {
		t.Fatal("first Escape must return to the compact options")
	}
	tt.Key(0, ui.KeyEscape)
	if a.modelMenu {
		t.Fatal("second Escape must close the picker")
	}
}

func TestStandardPickerProviderMemoryAndStaysOpen(t *testing.T) {
	a := testApp(t, "default")
	a.state.Settings.General.CompactModelPicker = false
	tt := ui.NewTester(a.View, 1320, 880)
	if err := tt.Click("Select model"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Sonnet 4.6"); err != nil {
		t.Fatal(err)
	}
	if !a.modelMenu || a.modelID != "claude-sonnet-4-6" {
		t.Fatal("standard picker should remain open after selecting a model")
	}
	if err := tt.Click("Codex"); err != nil {
		t.Fatal(err)
	}
	if a.selectedModel().ProviderID != "openai" {
		t.Fatal("provider tab retained the foreign model")
	}
	for _, row := range a.modelRows() {
		if row.provider.ID != "openai" {
			t.Fatal("standard picker mixed provider scopes")
		}
	}
	if err := tt.Click("Claude Code"); err != nil {
		t.Fatal(err)
	}
	if a.modelID != "claude-sonnet-4-6" {
		t.Fatalf("provider did not restore previous model: %s", a.modelID)
	}
	if err := tt.Click("Search models"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyEnter)
	if a.modelID != "claude-sonnet-4-6" {
		t.Fatal("keyboard highlight did not follow the provider's remembered model")
	}
	if err := tt.Click("Favorites"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("No starred models") {
		t.Fatal("empty favorites state missing")
	}
	tt.Key(0, ui.KeyEnter)
	if a.modelID != "claude-sonnet-4-6" {
		t.Fatal("Enter in an empty list changed the model")
	}
}

func TestCaptureModelCatalogAudit(t *testing.T) {
	dir := os.Getenv("ZERON_MODEL_AUDIT_DIR")
	if dir == "" {
		t.Skip("explicit audit capture directory required")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"light", "dark"} {
		for _, status := range []contracts.ModelCatalogStatus{contracts.CatalogLoading, contracts.CatalogError, contracts.CatalogEmpty, contracts.CatalogNoAgents} {
			a := testApp(t, "default")
			a.state.Settings.General.CompactModelPicker = false
			a.state.Settings.Appearance.Mode = mode
			a.state.Settings.Appearance.ReduceMotion = true
			a.state.Settings.Appearance.ThemeID = "zeron-" + mode
			a.state.ModelCatalog = contracts.ModelCatalogState{State: status, Error: "The model catalog could not be loaded."}
			if status == contracts.CatalogEmpty || status == contracts.CatalogNoAgents {
				a.state.Models = nil
				a.modelID = ""
			}
			tt := ui.NewTester(a.View, 1320, 880)
			if err := tt.Click("Select model"); err != nil {
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
		for _, compact := range []bool{true, false} {
			a := testApp(t, "default")
			a.selectSession("chat-0")
			a.files, a.right = true, true
			a.state.Settings.General.CompactModelPicker = compact
			tt := ui.NewTester(a.View, size[0], size[1])
			if err := tt.Click("Select model"); err != nil {
				t.Fatal(err)
			}
			r, ok := tt.Find("Model picker")
			if !ok || r.X < 0 || r.Y < 0 || r.X+r.W > float32(size[0]) || r.Y+r.H > float32(size[1]) {
				t.Fatalf("picker escaped %v compact=%v: %+v, open %v, selector %+v, texts %v", size, compact, r, a.modelMenu, func() ui.Rect { r, _ := tt.Find("Select model"); return r }(), tt.Texts())
			}
			want := float32(304)
			if compact {
				want = 256
			}
			if r.W != want {
				t.Fatalf("incorrect popup width: %v, want %v", r.W, want)
			}
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
	if err := tt.Click("Select model"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Change model"); err != nil {
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
	a.state.Settings.General.CompactModelPicker = false
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
	if err := tt.Click("Select model"); err != nil {
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
	if !a.modelNoAgents() || a.modelLabel() != "No agents available" {
		t.Fatal("all-disabled catalog remained a perpetual loading list")
	}
	a.selectSession("chat-0")
	if a.modelLabel() != "Opus 4.6" || len(a.modelRows()) == 0 {
		t.Fatal("existing chat lost its committed provider/model")
	}
}

func TestCompactModelPickerEffortAndSelection(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 1320, 880)
	if err := tt.Click("Select model"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Reasoning effort"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyEnd)
	if a.reasoning != "Max" {
		t.Fatalf("effort slider did not update model config: %s", a.reasoning)
	}
	if err := tt.Click("Change model"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("GPT-5.4"); err != nil {
		t.Fatal(err)
	}
	if a.modelID != "gpt-5.4" || a.reasoning == "Max" {
		t.Fatal("model selection retained unsupported reasoning")
	}
	if !a.modelMenu || a.modelListPage {
		t.Fatal("compact picker did not return to options")
	}
}

func TestModelFavoritesSearchAndPersistence(t *testing.T) {
	a := testApp(t, "default")
	m := a.state.Models[0]
	a.toggleModelFavorite(m)
	if !a.isModelFavorite(m) {
		t.Fatal("favorite was not persisted")
	}
	rows := a.modelRows()
	if rows[0].group != "favorites" || rows[0].model.ID != m.ID {
		t.Fatal("favorites did not precede provider groups")
	}
	a.modelQuery = "codex"
	for _, row := range a.modelRows() {
		if row.model.ProviderID != "openai" {
			t.Fatal("model search ignored provider name")
		}
	}
	a.toggleModelFavorite(m)
	if a.isModelFavorite(m) {
		t.Fatal("favorite removal did not persist")
	}
}

func TestModelPickerKeyboardFavoritesAndAccessMenu(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 1320, 880)
	if err := tt.Click("Access mode"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Plan mode"); err != nil {
		t.Fatal(err)
	}
	if !a.plan {
		t.Fatal("Plan menu did not update the run config")
	}
	tt.Key(0, ui.KeyEscape)
	if err := tt.Click("Select model"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Change model"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyDown)
	tt.Key(ui.Cmd|ui.Shift, ui.KeyF)
	if !a.isModelFavorite(a.state.Models[1]) {
		t.Fatal("keyboard favorite did not affect highlighted row")
	}
	tt.Key(ui.Cmd, ui.Key4)
	// The favorite section adds one row before the four provider rows.
	if a.modelID != "gpt-5.4" || a.modelListPage {
		t.Fatalf("number shortcut selected %s", a.modelID)
	}
	if a.state.Settings.ModelPicker.ProviderID != "openai" {
		t.Fatal("last selected provider was not saved")
	}
	if a.state.Settings.ModelPicker.Selections["openai"].ModelID != "gpt-5.4" {
		t.Fatal("provider model memory was not saved")
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
		a.selectModelProvider("openai")
		if a.selectedModel().ProviderID != provider {
			t.Fatal("provider changed while a turn was active")
		}
	}
}

func TestModelPickerPreservesShellFillAndProviderChoice(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 1320, 880)
	before := tt.Image().RGBAAt(800, 200)
	if err := tt.Click("Select model"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Change model"); err != nil {
		t.Fatal(err)
	}
	if got := tt.Image().RGBAAt(800, 200); got != before {
		t.Fatalf("model list changed the shell fill: %v -> %v", before, got)
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
	a.state.Settings.General.CompactModelPicker = false
	a.state.Settings.Appearance.ReduceMotion = true
	a.saveSettings()
	a.state.ModelCatalog = contracts.ModelCatalogState{State: contracts.CatalogLoading}
	tt := ui.NewTester(a.View, 900, 600)
	if err := tt.Click("Select model"); err != nil {
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
	if a.state.ModelCatalog.State != contracts.CatalogReady || !tt.HasText("Search models") {
		t.Fatalf("retry did not restore the catalog: %v error %q open %v texts %v", a.state.ModelCatalog, a.errorText, a.modelMenu, tt.Texts())
	}
	a.state.ModelCatalog = contracts.ModelCatalogState{State: contracts.CatalogNoAgents}
	for i := range a.state.Integrations.Providers {
		a.state.Integrations.Providers[i].Enabled = false
	}
	tt.Frame()
	if !tt.HasText("No agents available") || !tt.HasText("Enable an installed agent") {
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
