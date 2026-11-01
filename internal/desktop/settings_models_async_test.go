package desktop

import (
	"context"
	"testing"

	"gcode/internal/contracts"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestModelSettingsQueuedSaveKeepsNewerInputAndWaitsBeforeNavigation(t *testing.T) {
	app, core := runtimeSettingsTestApp(t)
	app.route, app.section = "settings", "Model settings"
	// Exercise the native command queue without attaching a graphics window.
	app.win = &mygo.Window{}
	t.Cleanup(func() { app.win = nil })
	app.addCustomProvider()
	id := app.modelsUI.selected
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Column(c).Fill().Padding(16).Children(func() { app.customProviderSettings(c) })
	}, 1000, 800)
	if id == "" || app.modelsUI.selected != id || !app.modelsUI.inlineSaving {
		t.Fatal("pending provider creation lost its selected draft")
	}
	complete := func(command contracts.Command) {
		t.Helper()
		err := core.Execute(context.Background(), command)
		app.commandFinished(command, err)
		if err != nil {
			t.Fatal(err)
		}
		snapshot, snapshotErr := core.Snapshot(context.Background())
		if snapshotErr != nil {
			t.Fatal(snapshotErr)
		}
		app.apply(snapshot)
		tt.Frame()
	}
	complete(<-app.jobs)
	if app.modelsUI.selected != id || !tt.HasText("Base URL") {
		t.Fatal("completed empty provider creation did not display its editor")
	}
	app.runtimeUI.provider.BaseURL = "https://first.example.com/v1"
	app.runtimeUI.key = "first-test-key"
	app.markInlineProviderDirty()
	app.saveInlineProvider()
	firstSave := <-app.jobs

	// Typing can continue while disk/network work is outstanding.
	app.runtimeUI.provider.BaseURL = "https://second.example.com/v1"
	app.runtimeUI.key = "second-test-key"
	app.markInlineProviderDirty()
	app.selectSettingsSection("Skills")
	complete(firstSave)
	if app.section != "Model settings" || !app.modelsUI.inlineDirty ||
		app.runtimeUI.key != "second-test-key" || app.runtimeUI.provider.BaseURL != "https://second.example.com/v1" {
		t.Fatal("an older save discarded newer input or navigated before saving it")
	}
	app.saveInlineProvider()
	complete(<-app.jobs)
	if app.section != "Skills" || app.runtimeUI.key != "" {
		t.Fatal("navigation did not resume after the current draft was saved")
	}
	snapshot, _ := core.Snapshot(context.Background())
	if len(snapshot.CustomProviders) != 1 || snapshot.CustomProviders[0].Provider.BaseURL != "https://second.example.com/v1" ||
		!snapshot.CustomProviders[0].HasAPIKey {
		t.Fatal("the newest queued draft did not reach the real core")
	}
}
