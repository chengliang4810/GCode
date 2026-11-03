package desktop

import (
	"context"
	"errors"
	"gcode/internal/assets"
	"reflect"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/i18n"
	localize "gcode/internal/i18n/view"
	"gcode/internal/mock"
	"github.com/egoist/mygo/ui"
)

func TestLanguageDefaultsAndSwitchesWithoutRestart(t *testing.T) {
	if err := assets.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	backend, err := mock.New(mock.Options{Instant: true})
	if err != nil {
		t.Fatal(err)
	}
	app, err := New(backend)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	if app.state.Settings.General.Language != i18n.Chinese {
		t.Fatal("new installation must default to Chinese")
	}
	app.draft = "Keep my English draft 设置"
	before := app.state.Messages
	app.OpenRoute("settings/General")
	tt := ui.NewTester(app.View, 1320, 1000)
	if _, ok := tt.Find("搜索设置"); ok {
		t.Fatal("settings search must not be shown")
	}
	for _, text := range []string{"通用", "语言", "简体中文", "发送消息按键"} {
		if !tt.HasText(text) {
			t.Fatalf("Chinese UI missing %q", text)
		}
	}
	if err := tt.Click("选择语言"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("English"); err != nil {
		t.Fatal(err)
	}
	if app.state.Settings.General.Language != i18n.English || !tt.HasText("General") || !tt.HasText("Send messages with") {
		t.Fatalf("live switch failed: %v", tt.Texts())
	}
	if err := tt.Click("Choose Language"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("简体中文"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("通用") || app.draft != "Keep my English draft 设置" || !reflect.DeepEqual(before, app.state.Messages) {
		t.Fatal("language switch lost page/draft or changed conversation content")
	}
	snapshot, err := backend.Snapshot(context.Background())
	if err != nil || snapshot.Settings.General.Language != i18n.Chinese {
		t.Fatalf("choice was not saved: %v", err)
	}
}

type failingLanguageService struct{ contracts.Service }

func (s failingLanguageService) Execute(ctx context.Context, command contracts.Command) error {
	if command.Kind == "settings.update" {
		return errors.New("settings unavailable")
	}
	return s.Service.Execute(ctx, command)
}

func TestLanguageSaveFailureRestoresRenderAndWorkspace(t *testing.T) {
	a := testApp(t, "default")
	a.service = failingLanguageService{a.service}
	a.state.Settings.General.Language = i18n.Chinese
	a.saveSettings()
	a.OpenRoute("settings/General")
	tt := ui.NewTester(a.View, 1320, 1000)
	if a.state.Settings.General.Language != i18n.English || !tt.HasText("General") {
		t.Fatal("failed save did not restore the prior language")
	}
	a.workspace.OpenSurface("changes")
	workspace := ui.NewTester(a.workspace.View, 600, 400)
	if !workspace.HasText("Changes") || workspace.HasText("更改") {
		t.Fatal("workspace language was not rolled back")
	}
}

func TestLanguageIsolationAndBusinessText(t *testing.T) {
	a := testApp(t, "default")
	b := testApp(t, "default")
	a.state.Settings.General.Language = i18n.Chinese
	a.OpenRoute("settings/General")
	b.OpenRoute("settings/General")
	zh := ui.NewTester(a.View, 1320, 1000)
	en := ui.NewTester(b.View, 1320, 1000)
	zh.Frame()
	if !zh.HasText("通用") || !en.HasText("General") {
		t.Fatal("language leaked between render roots")
	}
	// Interface helpers must leave arbitrary question/project labels untouched.
	for _, name := range []string{"Settings", "Cloud", "High", "Cancel"} {
		tt := ui.NewTester(func(c *ui.Context) {
			localize.Set(c, i18n.Chinese)
			textButton(c, name)
		}, 400, 100)
		if !tt.HasText(name) || tt.HasText(i18n.In(i18n.Chinese, name)) {
			t.Fatalf("business label %q was translated", name)
		}
	}
}
