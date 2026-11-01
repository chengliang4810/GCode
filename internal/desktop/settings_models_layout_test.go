package desktop

import (
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/design"
	localize "gcode/internal/i18n/view"
	"gcode/internal/model"
	"github.com/egoist/mygo/ui"
)

// These are implementation renders and geometry checks, not native-window or
// pixel-parity acceptance. The reference image contains branded providers;
// fixtures deliberately use only user-configured providers.
func TestModelSettingsReferenceLayout(t *testing.T) {
	for _, test := range []struct {
		name       string
		dark       bool
		configured bool
		width      int
		height     int
	}{
		{name: "empty-light", width: 1180, height: 980},
		{name: "configured-light", configured: true, width: 1180, height: 980},
		{name: "empty-dark", dark: true, width: 1180, height: 980},
		{name: "configured-dark", dark: true, configured: true, width: 1180, height: 980},
		{name: "narrow-light", width: 900, height: 800},
		{name: "narrow-dark", dark: true, width: 900, height: 800},
		{name: "wide-desktop", configured: true, width: 2048, height: 1230},
	} {
		t.Run(test.name, func(t *testing.T) {
			app, _ := runtimeSettingsTestApp(t)
			app.state.Settings.General.Language = "zh-CN"
			app.route, app.section = "settings", "Model settings"
			app.palette = design.Palette("rcode-light", false)
			app.state.Settings.Appearance.Mode = "Light"
			app.state.Settings.Appearance.ThemeID = "rcode-light"
			if test.dark {
				app.palette = design.Palette("rcode-dark", true)
				app.state.Settings.Appearance.Mode = "Dark"
				app.state.Settings.Appearance.ThemeID = "rcode-dark"
			}
			for i, name := range []string{"工作模型", "代码助手", "本地服务", "快速模型", "测试网关", "开发服务", "备用服务", "新供应商 2"} {
				provider := model.Provider{ID: "layout-" + string(rune('a'+i)), Name: name, Protocol: model.Messages}
				if i != 7 || test.configured {
					provider.BaseURL = "https://api.example.com/v1"
					provider.Models = []model.Model{{ID: "deep-model", ContextWindow: 200000, MaxOutput: 16000, ReasoningLevels: []string{"low", "high", "max"}}}
				}
				app.state.CustomProviders = append(app.state.CustomProviders, contracts.ProviderConfiguration{Provider: provider})
			}
			app.modelsUI.selected = "layout-h"
			tt := ui.NewTester(func(c *ui.Context) {
				if test.width <= 900 || test.name == "wide-desktop" {
					app.View(c)
					return
				}
				localize.Set(c, "zh-CN")
				c.SetTheme(app.palette.UI())
				c.Root().Font("Geist").FontSize(13).Background(app.palette.Background).TextColor(app.palette.Text)
				app.settingsView(c)
			}, test.width, test.height)

			labels := []string{"Model settings", "Add provider", "Base URL", "API format", "API key", "Add model"}
			bounds := map[string]ui.Rect{}
			for _, label := range labels {
				lookup := label
				switch label {
				case "Model settings":
					lookup = "Settings page heading"
				case "Base URL":
					lookup = "Provider Base URL"
				case "API format":
					lookup = "Provider API format"
				}
				target := app.t(lookup)
				if label == "Model settings" {
					target += ": " + app.t(label)
				}
				rect, ok := tt.Find(target)
				if !ok || rect.W <= 0 || rect.H <= 0 {
					t.Fatalf("missing model-settings control %q: %+v\n%v", label, rect, tt.Texts())
				}
				if rect.X < 0 || rect.Y < 0 || rect.X+rect.W > float32(test.width)+1 || rect.Y+rect.H > float32(test.height)+1 {
					t.Fatalf("model-settings control %q is outside viewport: %+v", label, rect)
				}
				bounds[label] = rect
			}
			base, protocol, key := bounds["Base URL"], bounds["API format"], bounds["API key"]
			if rightGap := bounds["Add provider"].X + bounds["Add provider"].W - base.X - base.W; rightGap > 56 {
				t.Fatalf("provider fields leave unused space on the right: %v pixels", rightGap)
			}
			if base.X < float32(test.width)*0.2 || base.Y >= protocol.Y || protocol.Y >= key.Y {
				t.Fatalf("provider fields must form one right-hand column: base=%+v protocol=%+v key=%+v", base, protocol, key)
			}
			if bounds["Add model"].Y <= key.Y {
				t.Fatal("model list appears before provider connection fields")
			}
			if bounds["Add provider"].X <= bounds["Model settings"].X {
				t.Fatal("add-provider action must be placed at the right of the page header")
			}
			if !test.configured && !tt.HasText(app.t("No models configured. Add a model to use this provider in chat.")) {
				t.Fatal("an empty provider must show the model-list empty state")
			}
			if err := tt.Click(app.t("Select provider") + " 工作模型"); err != nil {
				t.Fatal(err)
			}
			if !tt.HasText("deep-model") {
				t.Fatal("selecting a provider did not display its configured models")
			}
			var previous ui.Rect
			for _, action := range []string{"Test model", "Edit model", "Remove model"} {
				label := app.t(action) + " deep-model"
				r, ok := tt.Find(label)
				size := design.Controls().IconButtonSize
				if !ok || r.W != size || r.H != size {
					t.Fatalf("%s must use shared icon-button geometry: %+v", label, r)
				}
				if r.X < 0 || r.X+r.W > float32(test.width) || r.Y+r.H > float32(test.height) ||
					previous.W > 0 && (r.X < previous.X+previous.W || r.Y != previous.Y) {
					t.Fatalf("model actions overflow or overlap: previous=%+v current=%+v", previous, r)
				}
				previous = r
			}
			for _, text := range []string{"Test", "Edit", "Remove model"} {
				if slices.Contains(tt.Texts(), app.t(text)) {
					t.Fatalf("model row retained visible action text %q", text)
				}
			}
			if _, ok := tt.Find(app.t("Model row") + " deep-model"); !ok {
				t.Fatal("model row is not draggable")
			}
			if err := tt.Click(app.t("Select provider") + " 新供应商 2"); err != nil {
				t.Fatal(err)
			}
			if test.configured && !tt.HasText("deep-model") {
				t.Fatal("configured provider lost its model row on reselection")
			}
			if !test.configured && tt.HasText("deep-model") {
				t.Fatal("empty provider retained the previous provider's model row")
			}
			writeModelSettingsLayoutEvidence(t, tt, test.name, test.width, test.height, bounds)
		})
	}
}

func writeModelSettingsLayoutEvidence(t *testing.T, tt *ui.Tester, name string, width, height int, bounds map[string]ui.Rect) {
	t.Helper()
	directory := os.Getenv("RCODE_MODEL_SETTINGS_EVIDENCE")
	if directory == "" {
		return
	}
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, name)
	file, err := os.Create(path + ".png")
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, tt.Image()); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	metadata, err := json.MarshalIndent(map[string]any{
		"renderer": "MyGo ui.NewTester; no native-window or pixel-parity acceptance",
		"fixture":  "Eight user-configured providers; no built-in brand catalog and no external service requests.",
		"language": "zh-CN", "width": width, "height": height, "controls": bounds,
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".json", append(metadata, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
}
