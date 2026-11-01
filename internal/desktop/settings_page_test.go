package desktop

import (
	"encoding/json"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/model"
	"github.com/egoist/mygo/ui"
)

func TestSettingsControlsPageFrameAcrossSections(t *testing.T) {
	for _, test := range []struct {
		name          string
		width, height int
		dark, hidden  bool
	}{
		{name: "wide-light", width: 2048, height: 1230},
		{name: "wide-dark", width: 2048, height: 1230, dark: true},
		{name: "desktop-light", width: 1320, height: 880},
		{name: "desktop-dark", width: 1320, height: 880, dark: true},
		{name: "narrow-light", width: 900, height: 800},
		{name: "narrow-dark", width: 900, height: 800, dark: true},
		{name: "hidden-sidebar", width: 900, height: 800, hidden: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			app, _ := runtimeSettingsTestApp(t)
			app.route, app.sidebar = "settings", !test.hidden
			app.state.Settings.Appearance.Mode = "Light"
			app.state.Settings.Appearance.ThemeID = "rcode-light"
			if test.dark {
				app.state.Settings.Appearance.Mode = "Dark"
				app.state.Settings.Appearance.ThemeID = "rcode-dark"
			}
			app.state.CustomProviders = []contracts.ProviderConfiguration{{Provider: model.Provider{
				ID: "page-layout", Name: "Shared layout", Protocol: model.Chat,
				BaseURL: "https://example.com/v1", Models: []model.Model{{ID: "model"}},
			}}}
			var referenceContent, referenceHeading ui.Rect
			for _, section := range app.sections() {
				app.selectSettingsSection(section)
				tt := ui.NewTester(app.View, test.width, test.height)
				viewport, ok := tt.Find("Settings page viewport")
				if !ok {
					t.Fatalf("%s has no shared viewport", section)
				}
				content, ok := tt.Find("Settings page content")
				if !ok {
					t.Fatalf("%s has no shared content frame", section)
				}
				heading, ok := tt.Find("Settings page heading: " + section)
				if !ok {
					t.Fatalf("%s has no shared heading", section)
				}
				style := design.SettingsPage()
				expectedWidth := min(style.MaxWidth, viewport.W)
				expectedX := viewport.X + (viewport.W-expectedWidth)/2
				near := func(a, b float32) bool { return math.Abs(float64(a-b)) < 1 }
				if !near(content.W, expectedWidth) || !near(content.X, expectedX) ||
					!near(heading.X-content.X, style.PaddingX+style.HeadingInset) ||
					!near(heading.Y-content.Y, style.PaddingTop) {
					t.Fatalf("%s differs from the shared outer frame: viewport=%+v content=%+v heading=%+v",
						section, viewport, content, heading)
				}
				if section == "General" {
					referenceContent, referenceHeading = content, heading
				} else if !near(content.X, referenceContent.X) || !near(content.Y, referenceContent.Y) ||
					!near(content.W, referenceContent.W) || !near(heading.X, referenceHeading.X) ||
					!near(heading.Y, referenceHeading.Y) || !near(heading.H, referenceHeading.H) {
					t.Fatalf("%s does not align with General: content=%+v heading=%+v; General=%+v %+v",
						section, content, heading, referenceContent, referenceHeading)
				}
				if section == "General" || section == "Model settings" {
					writeSettingsPageFrameEvidence(t, tt, test.name, section, map[string]ui.Rect{
						"viewport": viewport, "content": content, "heading": heading,
					})
				}
			}
		})
	}
}

func writeSettingsPageFrameEvidence(t *testing.T, tt *ui.Tester, name, section string, bounds map[string]ui.Rect) {
	t.Helper()
	directory := os.Getenv("RCODE_SETTINGS_PAGE_EVIDENCE")
	if directory == "" {
		return
	}
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	suffix := "general"
	if section == "Model settings" {
		suffix = "models"
	}
	path := filepath.Join(directory, name+"-"+suffix)
	file, err := os.Create(path + ".png")
	if err != nil {
		t.Fatal(err)
	}
	err = png.Encode(file, tt.Image())
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("write layout evidence: %v %v", err, closeErr)
	}
	metadata, err := json.MarshalIndent(map[string]any{
		"renderer": "MyGo Tester full App.View, not native-window or pixel-parity acceptance",
		"fixture":  "Isolated local runtime and user-defined provider; no external model requests.",
		"section":  section, "bounds": bounds,
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".json", append(metadata, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
}
