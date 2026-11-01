package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"gcode/internal/contracts"
	localize "gcode/internal/i18n/view"
	"github.com/egoist/mygo/ui"
)

func TestSkillDetailsAndEditorUseSeparateModalDialogs(t *testing.T) {
	app, core := runtimeSettingsTestApp(t)
	skill := contracts.UserSkill{ID: "review", Name: "Review change", Description: "Check changes before release.", Content: "# Review\nInspect the working tree.", Enabled: true}
	app.dispatch(contracts.Command{Kind: "resource.skill.save", Resource: &contracts.ResourceInput{Skill: &skill}})
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Column(c).Fill().Padding(16).Gap(12).Children(func() { app.skillSettings(c) })
	}, 1000, 800)
	add, ok := tt.Find("Add skill")
	if !ok {
		t.Fatal("missing add action")
	}
	if err := tt.Click("Skill details Review change"); err != nil {
		t.Fatal(err)
	}
	if !app.capabilityUI.skillDetailsOpen || app.capabilityUI.skillOpen || !tt.HasText("Status") || !tt.HasText("SKILL.md") {
		t.Fatal("list did not open the read-only details dialog")
	}
	tt.ClickAt(add.X+add.W/2, add.Y+add.H/2)
	if app.capabilityUI.skillOpen {
		t.Fatal("details modal allowed an underlying add action")
	}
	if !app.capabilityUI.skillDetailsOpen {
		if err := tt.Click("Skill details Review change"); err != nil {
			t.Fatal(err)
		}
	}
	tt.Key(0, ui.KeyEscape)
	if app.capabilityUI.skillDetailsOpen {
		t.Fatal("Escape did not dismiss the details dialog")
	}
	if err := tt.Click("Skill details Review change"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Edit skill"); err != nil {
		t.Fatal(err)
	}
	if !app.capabilityUI.skillOpen || app.capabilityUI.skillDetailsOpen || tt.HasText("Import directory") {
		t.Fatal("editing reused the details or import form")
	}
	app.capabilityUI.skill.Content = "# Review\nRead and test every changed behavior."
	tt.Frame()
	if err := tt.Click("Save"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := core.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if app.capabilityUI.skillOpen || snapshot.UserResources.Skills[0].Content != app.capabilityUI.skill.Content {
		t.Fatalf("skill edit did not persist through the real core: %s", app.errorText)
	}
}

func TestSkillImportDialogPreservesMetadataAndSupportFiles(t *testing.T) {
	app, core := runtimeSettingsTestApp(t)
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "SKILL.md"), []byte("---\nname: local-review\ndescription: Imported review instructions\n---\n\nRead supporting examples."), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(directory, "examples"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "examples", "review.md"), []byte("Review the diff."), 0644); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Column(c).Fill().Padding(16).Gap(12).Children(func() { app.skillSettings(c) })
	}, 1000, 800)
	if err := tt.Click("Import skill"); err != nil {
		t.Fatal(err)
	}
	if !app.capabilityUI.skillImportOpen || app.capabilityUI.skillOpen || tt.HasText("SKILL.md") {
		t.Fatal("import action did not open its own directory dialog")
	}
	app.capabilityUI.skillImport = directory
	tt.Frame()
	if err := tt.Click("Import"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := core.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if app.errorText != "" || len(snapshot.UserResources.Skills) != 1 {
		t.Fatalf("directory import failed: %s", app.errorText)
	}
	got := snapshot.UserResources.Skills[0]
	if got.Name != "local-review" || got.Description != "Imported review instructions" || !slices.Contains(got.Files, "examples/review.md") {
		t.Fatalf("import lost skill metadata or support files: %+v", got)
	}
	if err := tt.Click("Skill details local-review"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("examples/review.md") || !tt.HasText("Imported review instructions") || got.Path == "" || !tt.HasText(got.Path) {
		t.Fatal("details omit imported description, host path or support files")
	}
}

type failingSkillSettingsService struct{ contracts.Service }

func (s failingSkillSettingsService) Execute(ctx context.Context, command contracts.Command) error {
	if command.Kind == "resource.skill.import" || command.Kind == "resource.skill.save" {
		return errors.New("skill storage unavailable")
	}
	return s.Service.Execute(ctx, command)
}

func TestSkillDialogSaveFailuresKeepTheCorrectDraft(t *testing.T) {
	for _, kind := range []string{"edit", "import"} {
		t.Run(kind, func(t *testing.T) {
			app, _ := runtimeSettingsTestApp(t)
			app.service = failingSkillSettingsService{app.service}
			action := "Save"
			if kind == "import" {
				app.importSkill()
				app.capabilityUI.skillImport = filepath.Join(t.TempDir(), "keep-directory")
				action = "Import"
			} else {
				app.editSkill(nil)
			}
			app.capabilityUI.skill.Name = "Keep my skill"
			app.capabilityUI.skill.Description = "Retain this unsaved description."
			tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(app.palette.UI()); app.skillSettings(c) }, 1000, 800)
			if err := tt.Click(action); err != nil {
				t.Fatal(err)
			}
			if app.errorText == "" || app.capabilityUI.skill.Name != "Keep my skill" || app.capabilityUI.skill.Description != "Retain this unsaved description." {
				t.Fatal("failed save lost its draft or error")
			}
			if kind == "import" && (!app.capabilityUI.skillImportOpen || app.capabilityUI.skillOpen) {
				t.Fatal("failed import reopened the editor instead of import")
			}
			if kind == "edit" && !app.capabilityUI.skillOpen {
				t.Fatal("failed save did not reopen the editor")
			}
		})
	}
}

func TestSkillDialogsRenderBothLanguages(t *testing.T) {
	directory := os.Getenv("RCODE_SETTINGS_EVIDENCE")
	if directory == "" {
		t.Skip("set RCODE_SETTINGS_EVIDENCE to record rendered evidence")
	}
	for _, language := range []string{"en", "zh-CN"} {
		t.Run(language, func(t *testing.T) {
			app, _ := runtimeSettingsTestApp(t)
			app.state.Settings.General.Language = language
			app.route, app.section = "settings", "Skills"
			app.state.UserResources.Skills = []contracts.UserSkill{{ID: "review", Name: "Review change", Description: "Check changes before release.", Content: "# Review\nRead and test changed behavior.", Enabled: true, Path: `C:\Users\user\.rcode\skills\review\SKILL.md`, Files: []string{"examples/review.md", "scripts/check.ps1"}}}
			tt := ui.NewTester(func(c *ui.Context) {
				localize.Set(c, language)
				c.SetTheme(app.palette.UI())
				ui.Row(c).Fill().Children(func() {
					ui.Column(c).Width(220).FillHeight().Children(func() { app.settingsNavigation(c) })
					app.settingsView(c)
				})
			}, 1100, 850)
			for _, mode := range []string{"details", "edit", "import"} {
				switch mode {
				case "details":
					app.showSkillDetails(app.state.UserResources.Skills[0])
				case "edit":
					app.editSkill(&app.state.UserResources.Skills[0])
				case "import":
					app.importSkill()
					app.capabilityUI.skillImport = `D:\skills\release-review`
				}
				tt.Frame()
				if err := os.MkdirAll(directory, 0755); err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(directory, "settings-skill-"+mode+"-"+language)
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
				data, _ := json.MarshalIndent(map[string]any{"renderer": "MyGo ui.NewTester; not native-window or visual parity acceptance", "language": language, "dialog": mode, "width": 1100, "height": 850}, "", "  ")
				if err := os.WriteFile(path+".json", append(data, '\n'), 0644); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
